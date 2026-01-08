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

// Package broker provides configuration for the credential broker sidecar
// that enables session tags (ABAC) mode for multi-tenant environments.
package broker

import (
	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
)

// Config holds the global configuration for broker mode
type Config struct {
	// Enabled indicates whether broker mode is available
	Enabled bool
	// DefaultBrokerImage is the default container image for the broker sidecar
	DefaultBrokerImage string
	// DefaultCredentialsPath is the default mount path for credentials
	DefaultCredentialsPath string
	// Region is the AWS region for STS calls
	Region string
	// STSEndpoint is an optional custom STS endpoint (for testing)
	STSEndpoint string
}

// NewConfig creates a new broker configuration with defaults
func NewConfig() *Config {
	return &Config{
		Enabled:                false,
		DefaultBrokerImage:     pkg.DefaultBrokerImage,
		DefaultCredentialsPath: pkg.DefaultCredentialsPath,
	}
}

// PatchConfig holds the per-pod configuration for broker injection
type PatchConfig struct {
	// BaseRoleARN is the IRSA role to assume via AssumeRoleWithWebIdentity
	BaseRoleARN string
	// TargetRoleARN is the shared role to assume with session tags
	TargetRoleARN string
	// SessionTags is a comma-separated list of key=value pairs
	SessionTags string
	// BrokerImage is the container image for the broker sidecar
	BrokerImage string
	// CredentialsPath is the mount path for the credentials file
	CredentialsPath string
	// Region is the AWS region for STS calls
	Region string
	// STSEndpoint is an optional custom STS endpoint
	STSEndpoint string
	// TokenMountPath is where the IRSA token is mounted
	TokenMountPath string
	// TokenExpiration is the token expiration in seconds
	TokenExpiration int64
	// Audience is the token audience
	Audience string
}

// IsBrokerMode returns true if broker mode should be used for this configuration
func (p *PatchConfig) IsBrokerMode() bool {
	return p != nil && p.TargetRoleARN != ""
}
