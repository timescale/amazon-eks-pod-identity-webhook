/*
Copyright 2024 Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License").
You may not use this file except in compliance with the License.
A copy of the License is located at

	http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed
on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
express or implied. See the License for the specific language governing
permissions and limitations under the License.
*/

package credentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCredentialWriter(t *testing.T) {
	path := "/tmp/test/credentials"
	writer := NewCredentialWriter(path)

	assert.NotNil(t, writer)
	assert.Equal(t, path, writer.path)
}

func TestCredentialWriter_Write(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "broker-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	credPath := filepath.Join(tmpDir, "credentials")
	writer := NewCredentialWriter(credPath)

	creds := &Credentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBYaDEXAMPLETOKEN",
		Expiration:      time.Now().Add(1 * time.Hour),
	}

	err = writer.Write(creds)
	require.NoError(t, err)

	// Verify file exists
	info, err := os.Stat(credPath)
	require.NoError(t, err)
	assert.False(t, info.IsDir())

	// Verify file permissions (0600)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Verify file contents
	content, err := os.ReadFile(credPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "[default]")
	assert.Contains(t, contentStr, "aws_access_key_id = AKIAIOSFODNN7EXAMPLE")
	assert.Contains(t, contentStr, "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	assert.Contains(t, contentStr, "aws_session_token = FwoGZXIvYXdzEBYaDEXAMPLETOKEN")
}

func TestCredentialWriter_Write_CreatesDirectory(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "broker-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Nested path that doesn't exist
	credPath := filepath.Join(tmpDir, "nested", "path", "credentials")
	writer := NewCredentialWriter(credPath)

	creds := &Credentials{
		AccessKeyID:     "AKIATEST",
		SecretAccessKey: "secretkey",
		SessionToken:    "token",
		Expiration:      time.Now().Add(1 * time.Hour),
	}

	err = writer.Write(creds)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(credPath)
	require.NoError(t, err)
}

func TestCredentialWriter_Write_AtomicUpdate(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "broker-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	credPath := filepath.Join(tmpDir, "credentials")
	writer := NewCredentialWriter(credPath)

	// Write initial credentials
	creds1 := &Credentials{
		AccessKeyID:     "AKIAOLD",
		SecretAccessKey: "oldsecret",
		SessionToken:    "oldtoken",
		Expiration:      time.Now().Add(1 * time.Hour),
	}
	err = writer.Write(creds1)
	require.NoError(t, err)

	// Write new credentials
	creds2 := &Credentials{
		AccessKeyID:     "AKIANEW",
		SecretAccessKey: "newsecret",
		SessionToken:    "newtoken",
		Expiration:      time.Now().Add(1 * time.Hour),
	}
	err = writer.Write(creds2)
	require.NoError(t, err)

	// Verify file contains new credentials
	content, err := os.ReadFile(credPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "AKIANEW")
	assert.Contains(t, contentStr, "newsecret")
	assert.Contains(t, contentStr, "newtoken")
	assert.NotContains(t, contentStr, "AKIAOLD")

	// Verify no temp file remains
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	for _, f := range files {
		assert.False(t, strings.HasSuffix(f.Name(), ".tmp"), "Temp file should not remain")
	}
}

func TestCredentialWriter_Write_Format(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "broker-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	credPath := filepath.Join(tmpDir, "credentials")
	writer := NewCredentialWriter(credPath)

	creds := &Credentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "AQoDYXdzEJr...",
		Expiration:      time.Now().Add(1 * time.Hour),
	}

	err = writer.Write(creds)
	require.NoError(t, err)

	content, err := os.ReadFile(credPath)
	require.NoError(t, err)

	// Verify exact format that AWS SDKs expect
	expected := `[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
aws_session_token = AQoDYXdzEJr...
`
	assert.Equal(t, expected, string(content))
}
