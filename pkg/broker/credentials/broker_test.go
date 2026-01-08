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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ValidConfig(t *testing.T) {
	cfg := Config{
		TokenPath:       "/path/to/token",
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/target-role",
		CredentialsPath: "/path/to/credentials",
		SessionName:     "test-session",
		RefreshBuffer:   5 * time.Minute,
		SessionDuration: 1 * time.Hour,
		Region:          "us-west-2",
	}

	broker, err := New(cfg)

	require.NoError(t, err)
	assert.NotNil(t, broker)
}

func TestNew_InvalidBaseRoleARN(t *testing.T) {
	cfg := Config{
		TokenPath:       "/path/to/token",
		BaseRoleARN:     "invalid-arn",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/target-role",
		CredentialsPath: "/path/to/credentials",
	}

	broker, err := New(cfg)

	assert.Error(t, err)
	assert.Nil(t, broker)
	assert.Contains(t, err.Error(), "BaseRoleARN has invalid format")
}

func TestNew_InvalidTargetRoleARN(t *testing.T) {
	cfg := Config{
		TokenPath:       "/path/to/token",
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "not-a-valid-arn",
		CredentialsPath: "/path/to/credentials",
	}

	broker, err := New(cfg)

	assert.Error(t, err)
	assert.Nil(t, broker)
	assert.Contains(t, err.Error(), "TargetRoleARN has invalid format")
}

func TestNew_SessionNameTruncation(t *testing.T) {
	longName := "this-is-a-very-long-session-name-that-exceeds-the-sixty-four-character-aws-limit-for-session-names"
	cfg := Config{
		TokenPath:       "/path/to/token",
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/target-role",
		CredentialsPath: "/path/to/credentials",
		SessionName:     longName,
	}

	broker, err := New(cfg)

	require.NoError(t, err)
	assert.NotNil(t, broker)
	assert.LessOrEqual(t, len(broker.config.SessionName), 64)
}

func TestNew_DefaultValues(t *testing.T) {
	cfg := Config{
		TokenPath:       "/path/to/token",
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/target-role",
		CredentialsPath: "/path/to/credentials",
		// RefreshBuffer, SessionDuration, and SessionName are not set
	}

	broker, err := New(cfg)

	require.NoError(t, err)
	assert.NotNil(t, broker)
	assert.Equal(t, DefaultRefreshBuffer, broker.config.RefreshBuffer)
	assert.Equal(t, DefaultSessionDuration, broker.config.SessionDuration)
	assert.Equal(t, "eks-broker", broker.config.SessionName)
}

func TestNew_MissingTokenPath(t *testing.T) {
	cfg := Config{
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/target-role",
		CredentialsPath: "/path/to/credentials",
	}

	broker, err := New(cfg)

	assert.Error(t, err)
	assert.Nil(t, broker)
	assert.Contains(t, err.Error(), "TokenPath is required")
}

func TestNew_MissingBaseRoleARN(t *testing.T) {
	cfg := Config{
		TokenPath:       "/path/to/token",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/target-role",
		CredentialsPath: "/path/to/credentials",
	}

	broker, err := New(cfg)

	assert.Error(t, err)
	assert.Nil(t, broker)
	assert.Contains(t, err.Error(), "BaseRoleARN is required")
}

func TestNew_MissingTargetRoleARN(t *testing.T) {
	cfg := Config{
		TokenPath:       "/path/to/token",
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		CredentialsPath: "/path/to/credentials",
	}

	broker, err := New(cfg)

	assert.Error(t, err)
	assert.Nil(t, broker)
	assert.Contains(t, err.Error(), "TargetRoleARN is required")
}

func TestNew_MissingCredentialsPath(t *testing.T) {
	cfg := Config{
		TokenPath:     "/path/to/token",
		BaseRoleARN:   "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN: "arn:aws:iam::123456789012:role/target-role",
	}

	broker, err := New(cfg)

	assert.Error(t, err)
	assert.Nil(t, broker)
	assert.Contains(t, err.Error(), "CredentialsPath is required")
}

func TestNew_WithSessionTags(t *testing.T) {
	cfg := Config{
		TokenPath:       "/path/to/token",
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/target-role",
		CredentialsPath: "/path/to/credentials",
		SessionTags: map[string]string{
			"tenant-id":  "abc123",
			"project-id": "xyz789",
			"namespace":  "production",
		},
	}

	broker, err := New(cfg)

	require.NoError(t, err)
	assert.NotNil(t, broker)
	assert.Len(t, broker.config.SessionTags, 3)
	assert.Equal(t, "abc123", broker.config.SessionTags["tenant-id"])
	assert.Equal(t, "xyz789", broker.config.SessionTags["project-id"])
	assert.Equal(t, "production", broker.config.SessionTags["namespace"])
}

func TestConfig_AllFields(t *testing.T) {
	cfg := Config{
		TokenPath:       "/var/run/secrets/token",
		BaseRoleARN:     "arn:aws:iam::111111111111:role/base",
		TargetRoleARN:   "arn:aws:iam::222222222222:role/target",
		SessionTags:     map[string]string{"key": "value"},
		CredentialsPath: "/var/run/credentials",
		SessionName:     "my-session",
		RefreshBuffer:   10 * time.Minute,
		SessionDuration: 2 * time.Hour,
		Region:          "eu-central-1",
	}

	assert.Equal(t, "/var/run/secrets/token", cfg.TokenPath)
	assert.Equal(t, "arn:aws:iam::111111111111:role/base", cfg.BaseRoleARN)
	assert.Equal(t, "arn:aws:iam::222222222222:role/target", cfg.TargetRoleARN)
	assert.Equal(t, map[string]string{"key": "value"}, cfg.SessionTags)
	assert.Equal(t, "/var/run/credentials", cfg.CredentialsPath)
	assert.Equal(t, "my-session", cfg.SessionName)
	assert.Equal(t, 10*time.Minute, cfg.RefreshBuffer)
	assert.Equal(t, 2*time.Hour, cfg.SessionDuration)
	assert.Equal(t, "eu-central-1", cfg.Region)
}

func TestCredentials_Fields(t *testing.T) {
	expiration := time.Now().Add(1 * time.Hour)
	creds := &Credentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBY...",
		Expiration:      expiration,
	}

	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", creds.AccessKeyID)
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", creds.SecretAccessKey)
	assert.Equal(t, "FwoGZXIvYXdzEBY...", creds.SessionToken)
	assert.Equal(t, expiration, creds.Expiration)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, 5*time.Minute, DefaultRefreshBuffer)
	assert.Equal(t, 1*time.Hour, DefaultSessionDuration)
	assert.Equal(t, 30*time.Second, MinRefreshInterval)
}
