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

package broker

import (
	"testing"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"github.com/stretchr/testify/assert"
)

func TestNewConfig(t *testing.T) {
	config := NewConfig()

	assert.NotNil(t, config)
	assert.False(t, config.Enabled)
	assert.Equal(t, pkg.DefaultBrokerImage, config.DefaultBrokerImage)
	assert.Equal(t, pkg.DefaultCredentialsPath, config.DefaultCredentialsPath)
	assert.Empty(t, config.Region)
	assert.Empty(t, config.STSEndpoint)
}

func TestConfig_Fields(t *testing.T) {
	config := &Config{
		Enabled:                true,
		DefaultBrokerImage:     "my-custom-broker:v1",
		DefaultCredentialsPath: "/custom/path",
		Region:                 "eu-west-1",
		STSEndpoint:            "http://localhost:9999",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "my-custom-broker:v1", config.DefaultBrokerImage)
	assert.Equal(t, "/custom/path", config.DefaultCredentialsPath)
	assert.Equal(t, "eu-west-1", config.Region)
	assert.Equal(t, "http://localhost:9999", config.STSEndpoint)
}

func TestPatchConfig_IsBrokerMode_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		config   *PatchConfig
		expected bool
	}{
		{
			name:     "nil config returns false",
			config:   nil,
			expected: false,
		},
		{
			name:     "empty config returns false",
			config:   &PatchConfig{},
			expected: false,
		},
		{
			name: "only base role returns false",
			config: &PatchConfig{
				BaseRoleARN: "arn:aws:iam::123456789012:role/base",
			},
			expected: false,
		},
		{
			name: "only target role returns true",
			config: &PatchConfig{
				TargetRoleARN: "arn:aws:iam::123456789012:role/target",
			},
			expected: true,
		},
		{
			name: "both roles returns true",
			config: &PatchConfig{
				BaseRoleARN:   "arn:aws:iam::123456789012:role/base",
				TargetRoleARN: "arn:aws:iam::123456789012:role/target",
			},
			expected: true,
		},
		{
			name: "full config returns true",
			config: &PatchConfig{
				BaseRoleARN:     "arn:aws:iam::123456789012:role/base",
				TargetRoleARN:   "arn:aws:iam::123456789012:role/target",
				SessionTags:     "key=value",
				BrokerImage:     "image:tag",
				CredentialsPath: "/path",
				Region:          "us-east-1",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsBrokerMode()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPatchConfig_AllFields(t *testing.T) {
	config := &PatchConfig{
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/target",
		SessionTags:     "tenant=abc,project=xyz",
		BrokerImage:     "broker:v1.0.0",
		CredentialsPath: "/var/run/creds",
		Region:          "ap-southeast-1",
		STSEndpoint:     "https://sts.ap-southeast-1.amazonaws.com",
		TokenMountPath:  "/var/run/secrets/token",
		TokenExpiration: 3600,
		Audience:        "custom-audience",
	}

	assert.Equal(t, "arn:aws:iam::123456789012:role/base", config.BaseRoleARN)
	assert.Equal(t, "arn:aws:iam::123456789012:role/target", config.TargetRoleARN)
	assert.Equal(t, "tenant=abc,project=xyz", config.SessionTags)
	assert.Equal(t, "broker:v1.0.0", config.BrokerImage)
	assert.Equal(t, "/var/run/creds", config.CredentialsPath)
	assert.Equal(t, "ap-southeast-1", config.Region)
	assert.Equal(t, "https://sts.ap-southeast-1.amazonaws.com", config.STSEndpoint)
	assert.Equal(t, "/var/run/secrets/token", config.TokenMountPath)
	assert.Equal(t, int64(3600), config.TokenExpiration)
	assert.Equal(t, "custom-audience", config.Audience)
	assert.True(t, config.IsBrokerMode())
}
