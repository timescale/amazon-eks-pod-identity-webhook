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
	"encoding/json"
	"testing"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGeneratePatch_NilConfig(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	patches, changed := GeneratePatch(pod, nil, nil)
	assert.Nil(t, patches)
	assert.False(t, changed)
}

func TestGeneratePatch_NotBrokerMode(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	config := &PatchConfig{
		BaseRoleARN: "arn:aws:iam::123456789012:role/base-role",
		// TargetRoleARN is empty, so not broker mode
	}

	patches, changed := GeneratePatch(pod, config, nil)
	assert.Nil(t, patches)
	assert.False(t, changed)
}

func TestGeneratePatch_BrokerMode(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "my-app:latest",
				},
			},
		},
	}

	config := &PatchConfig{
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/shared-role",
		SessionTags:     "tenant-id=abc123,project-id=xyz",
		BrokerImage:     "my-broker:latest",
		CredentialsPath: "/var/run/aws-credentials",
		TokenMountPath:  "/var/run/secrets/eks.amazonaws.com/serviceaccount",
		TokenExpiration: 86400,
		Audience:        "sts.amazonaws.com",
	}

	patches, changed := GeneratePatch(pod, config, nil)

	require.NotNil(t, patches)
	assert.True(t, changed)
	assert.Greater(t, len(patches), 0)

	// Verify we have volume patches
	hasCredentialsVolume := false
	hasTokenVolume := false
	hasBrokerContainer := false
	hasContainerEnv := false

	for _, patch := range patches {
		if patch.Path == "/spec/volumes" || patch.Path == "/spec/volumes/-" {
			// Check if it's the credentials or token volume
			if vol, ok := patch.Value.(corev1.Volume); ok {
				if vol.Name == pkg.CredentialsVolumeName {
					hasCredentialsVolume = true
				}
				if vol.Name == pkg.BrokerTokenVolumeName {
					hasTokenVolume = true
				}
			}
			if vols, ok := patch.Value.([]corev1.Volume); ok {
				for _, vol := range vols {
					if vol.Name == pkg.CredentialsVolumeName {
						hasCredentialsVolume = true
					}
					if vol.Name == pkg.BrokerTokenVolumeName {
						hasTokenVolume = true
					}
				}
			}
		}
		if patch.Path == "/spec/containers/-" {
			if container, ok := patch.Value.(corev1.Container); ok {
				if container.Name == pkg.BrokerContainerName {
					hasBrokerContainer = true
					// Verify broker container has correct env vars
					for _, env := range container.Env {
						if env.Name == "BROKER_BASE_ROLE_ARN" {
							assert.Equal(t, config.BaseRoleARN, env.Value)
						}
						if env.Name == "BROKER_TARGET_ROLE_ARN" {
							assert.Equal(t, config.TargetRoleARN, env.Value)
						}
					}
				}
			}
		}
		if patch.Path == "/spec/containers/0/env/-" || patch.Path == "/spec/containers/0/env" {
			hasContainerEnv = true
		}
	}

	assert.True(t, hasCredentialsVolume, "Should have credentials volume")
	assert.True(t, hasTokenVolume, "Should have token volume")
	assert.True(t, hasBrokerContainer, "Should have broker container")
	assert.True(t, hasContainerEnv, "Should have container env patches")
}

func TestGeneratePatch_SkipContainers(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "my-app:latest",
				},
				{
					Name:  "sidecar",
					Image: "sidecar:latest",
				},
			},
		},
	}

	config := &PatchConfig{
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/shared-role",
		BrokerImage:     "my-broker:latest",
		CredentialsPath: "/var/run/aws-credentials",
	}

	containersToSkip := map[string]bool{
		"sidecar": true,
	}

	patches, changed := GeneratePatch(pod, config, containersToSkip)

	require.NotNil(t, patches)
	assert.True(t, changed)

	// Verify sidecar container doesn't get env patches
	for _, patch := range patches {
		// Container index 1 is sidecar, it should not have env/volumeMount patches
		assert.NotContains(t, patch.Path, "/spec/containers/1/env")
		assert.NotContains(t, patch.Path, "/spec/containers/1/volumeMounts")
	}
}

func TestGeneratePatch_ExistingVolume(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "my-app:latest",
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: pkg.CredentialsVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	config := &PatchConfig{
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/shared-role",
		BrokerImage:     "my-broker:latest",
		CredentialsPath: "/var/run/aws-credentials",
	}

	patches, changed := GeneratePatch(pod, config, nil)

	require.NotNil(t, patches)
	assert.True(t, changed)

	// Verify we don't add duplicate credentials volume
	credVolCount := 0
	for _, patch := range patches {
		if patch.Path == "/spec/volumes/-" {
			if vol, ok := patch.Value.(corev1.Volume); ok {
				if vol.Name == pkg.CredentialsVolumeName {
					credVolCount++
				}
			}
		}
	}
	assert.Equal(t, 0, credVolCount, "Should not add duplicate credentials volume")
}

func TestGeneratePatch_SessionTagsParsing(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "my-namespace",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "my-app:latest",
				},
			},
		},
	}

	config := &PatchConfig{
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/shared-role",
		SessionTags:     "tenant-id=abc123,project-id=xyz789",
		BrokerImage:     "my-broker:latest",
		CredentialsPath: "/var/run/aws-credentials",
	}

	patches, _ := GeneratePatch(pod, config, nil)

	// Find the broker container patch and verify session tags are parsed correctly
	for _, patch := range patches {
		if patch.Path == "/spec/containers/-" {
			if container, ok := patch.Value.(corev1.Container); ok {
				if container.Name == pkg.BrokerContainerName {
					envMap := make(map[string]string)
					for _, env := range container.Env {
						envMap[env.Name] = env.Value
					}

					// Verify session tags are converted to env vars
					assert.Equal(t, "abc123", envMap["BROKER_TAG_TENANT_ID"])
					assert.Equal(t, "xyz789", envMap["BROKER_TAG_PROJECT_ID"])
					// Verify namespace is always added
					assert.Equal(t, "my-namespace", envMap["BROKER_TAG_NAMESPACE"])
				}
			}
		}
	}
}

func TestPatchConfig_IsBrokerMode(t *testing.T) {
	tests := []struct {
		name     string
		config   *PatchConfig
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name: "no target role ARN",
			config: &PatchConfig{
				BaseRoleARN: "arn:aws:iam::123456789012:role/base-role",
			},
			expected: false,
		},
		{
			name: "with target role ARN",
			config: &PatchConfig{
				BaseRoleARN:   "arn:aws:iam::123456789012:role/base-role",
				TargetRoleARN: "arn:aws:iam::123456789012:role/shared-role",
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

func TestGeneratePatch_JSONSerializable(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "my-app:latest",
				},
			},
		},
	}

	config := &PatchConfig{
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/shared-role",
		SessionTags:     "tenant-id=abc123",
		BrokerImage:     "my-broker:latest",
		CredentialsPath: "/var/run/aws-credentials",
		Region:          "us-west-2",
	}

	patches, _ := GeneratePatch(pod, config, nil)

	// Verify patches can be serialized to JSON (required for admission webhook)
	jsonBytes, err := json.Marshal(patches)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonBytes)

	// Verify it can be deserialized
	var deserializedPatches []PatchOperation
	err = json.Unmarshal(jsonBytes, &deserializedPatches)
	require.NoError(t, err)
	assert.Equal(t, len(patches), len(deserializedPatches))
}

func TestGeneratePatch_InitContainers(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:  "init",
					Image: "init:latest",
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "my-app:latest",
				},
			},
		},
	}

	config := &PatchConfig{
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/shared-role",
		BrokerImage:     "my-broker:latest",
		CredentialsPath: "/var/run/aws-credentials",
	}

	patches, changed := GeneratePatch(pod, config, nil)

	require.NotNil(t, patches)
	assert.True(t, changed)

	// Verify init containers get patched too
	hasInitContainerPatch := false
	for _, patch := range patches {
		if patch.Path == "/spec/initContainers/0/env" || patch.Path == "/spec/initContainers/0/env/-" {
			hasInitContainerPatch = true
		}
	}
	assert.True(t, hasInitContainerPatch, "Init containers should be patched")
}

func TestGeneratePatch_InvalidSessionTags(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "my-app:latest",
				},
			},
		},
	}

	config := &PatchConfig{
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/shared-role",
		SessionTags:     "valid-key=valid-value,invalid<key>=value,key=invalid<value>",
		BrokerImage:     "my-broker:latest",
		CredentialsPath: "/var/run/aws-credentials",
	}

	patches, _ := GeneratePatch(pod, config, nil)

	// Find the broker container and verify only valid tags are included
	for _, patch := range patches {
		if patch.Path == "/spec/containers/-" {
			if container, ok := patch.Value.(corev1.Container); ok {
				if container.Name == pkg.BrokerContainerName {
					envMap := make(map[string]string)
					for _, env := range container.Env {
						envMap[env.Name] = env.Value
					}

					// Valid tag should be present
					assert.Equal(t, "valid-value", envMap["BROKER_TAG_VALID_KEY"])
					// Invalid tags should NOT be present
					_, hasInvalidKey := envMap["BROKER_TAG_INVALID<KEY>"]
					assert.False(t, hasInvalidKey, "Invalid key should be skipped")
				}
			}
		}
	}
}

func TestGeneratePatch_RegionAndSTSEndpoint(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "my-app:latest",
				},
			},
		},
	}

	config := &PatchConfig{
		BaseRoleARN:     "arn:aws:iam::123456789012:role/base-role",
		TargetRoleARN:   "arn:aws:iam::123456789012:role/shared-role",
		BrokerImage:     "my-broker:latest",
		CredentialsPath: "/var/run/aws-credentials",
		Region:          "us-west-2",
		STSEndpoint:     "http://localhost:9999",
	}

	patches, _ := GeneratePatch(pod, config, nil)

	// Find the broker container and verify region/sts endpoint are set
	for _, patch := range patches {
		if patch.Path == "/spec/containers/-" {
			if container, ok := patch.Value.(corev1.Container); ok {
				if container.Name == pkg.BrokerContainerName {
					envMap := make(map[string]string)
					for _, env := range container.Env {
						envMap[env.Name] = env.Value
					}

					assert.Equal(t, "us-west-2", envMap["AWS_REGION"])
					assert.Equal(t, "us-west-2", envMap["AWS_DEFAULT_REGION"])
					assert.Equal(t, "http://localhost:9999", envMap["AWS_ENDPOINT_URL_STS"])
				}
			}
		}
	}
}
