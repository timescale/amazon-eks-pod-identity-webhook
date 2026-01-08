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
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"
)

// CredentialWriter writes AWS credentials to a file in the standard format
type CredentialWriter struct {
	path string
}

// NewCredentialWriter creates a new CredentialWriter
func NewCredentialWriter(path string) *CredentialWriter {
	return &CredentialWriter{path: path}
}

// Write writes credentials to the configured file path
func (w *CredentialWriter) Write(creds *Credentials) error {
	// Ensure directory exists
	dir := filepath.Dir(w.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	// Format credentials in AWS credentials file format
	content := fmt.Sprintf(`[default]
aws_access_key_id = %s
aws_secret_access_key = %s
aws_session_token = %s
`, creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken)

	// Write to a temporary file first, then rename for atomicity
	tmpPath := w.path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write temporary credentials file: %w", err)
	}

	// Atomically replace the credentials file
	if err := os.Rename(tmpPath, w.path); err != nil {
		os.Remove(tmpPath) // Clean up on failure
		return fmt.Errorf("failed to rename credentials file: %w", err)
	}

	klog.V(3).Infof("Wrote credentials to %s", w.path)
	return nil
}
