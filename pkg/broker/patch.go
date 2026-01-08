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
	"fmt"
	"strings"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
)

// PatchOperation represents a JSON patch operation
type PatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

// GeneratePatch generates the JSON patch operations to inject the broker sidecar
func GeneratePatch(pod *corev1.Pod, config *PatchConfig, containersToSkip map[string]bool) ([]PatchOperation, bool) {
	if config == nil || !config.IsBrokerMode() {
		return nil, false
	}

	klog.V(3).Infof("Generating broker patch for pod %s/%s (baseRole=%s, targetRole=%s)",
		pod.Namespace, pod.Name, config.BaseRoleARN, config.TargetRoleARN)

	var patches []PatchOperation
	changed := false

	// Add volumes
	volumePatches, volumeChanged := createVolumePatches(pod, config)
	patches = append(patches, volumePatches...)
	if volumeChanged {
		changed = true
	}

	// Add broker sidecar container
	sidecarPatches := createSidecarPatch(pod, config)
	patches = append(patches, sidecarPatches...)
	changed = true

	// Modify existing containers to use credentials file
	containerPatches, containerChanged := createContainerPatches(pod, config, containersToSkip)
	patches = append(patches, containerPatches...)
	if containerChanged {
		changed = true
	}

	return patches, changed
}

func createVolumePatches(pod *corev1.Pod, config *PatchConfig) ([]PatchOperation, bool) {
	var patches []PatchOperation
	changed := false

	// Check if volumes already exist
	credVolExists := false
	irsaVolExists := false
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == pkg.CredentialsVolumeName {
			credVolExists = true
		}
		if vol.Name == pkg.BrokerTokenVolumeName {
			irsaVolExists = true
		}
	}

	var volumes []corev1.Volume

	// Add credentials volume (emptyDir for sharing credentials between sidecar and app)
	if !credVolExists {
		volumes = append(volumes, corev1.Volume{
			Name: pkg.CredentialsVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumMemory,
				},
			},
		})
		changed = true
	}

	// Add IRSA token volume if not present (projected service account token)
	if !irsaVolExists {
		expirationSeconds := config.TokenExpiration
		if expirationSeconds == 0 {
			expirationSeconds = pkg.DefaultTokenExpiration
		}
		audience := config.Audience
		if audience == "" {
			audience = "sts.amazonaws.com"
		}
		volumes = append(volumes, corev1.Volume{
			Name: pkg.BrokerTokenVolumeName,
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Audience:          audience,
								ExpirationSeconds: &expirationSeconds,
								Path:              pkg.DefaultIRSATokenFile,
							},
						},
					},
				},
			},
		})
		changed = true
	}

	if len(volumes) > 0 {
		if pod.Spec.Volumes == nil {
			patches = append(patches, PatchOperation{
				Op:    "add",
				Path:  "/spec/volumes",
				Value: volumes,
			})
		} else {
			for _, vol := range volumes {
				patches = append(patches, PatchOperation{
					Op:    "add",
					Path:  "/spec/volumes/-",
					Value: vol,
				})
			}
		}
	}

	return patches, changed
}

func createSidecarPatch(pod *corev1.Pod, config *PatchConfig) []PatchOperation {
	credentialsPath := config.CredentialsPath
	if credentialsPath == "" {
		credentialsPath = pkg.DefaultCredentialsPath
	}

	tokenMountPath := config.TokenMountPath
	if tokenMountPath == "" {
		tokenMountPath = pkg.DefaultIRSATokenPath
	}

	// Build environment variables
	env := []corev1.EnvVar{
		{
			Name:  "BROKER_BASE_ROLE_ARN",
			Value: config.BaseRoleARN,
		},
		{
			Name:  "BROKER_TARGET_ROLE_ARN",
			Value: config.TargetRoleARN,
		},
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
	}

	// Add session tags as environment variables
	if config.SessionTags != "" {
		for _, pair := range strings.Split(config.SessionTags, ",") {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				value := strings.TrimSpace(kv[1])
				envKey := "BROKER_TAG_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
				env = append(env, corev1.EnvVar{
					Name:  envKey,
					Value: value,
				})
			}
		}
	}

	// Always add namespace as a session tag
	env = append(env, corev1.EnvVar{
		Name:  "BROKER_TAG_NAMESPACE",
		Value: pod.Namespace,
	})

	// Add region if configured
	if config.Region != "" {
		env = append(env, corev1.EnvVar{
			Name:  "AWS_REGION",
			Value: config.Region,
		})
		env = append(env, corev1.EnvVar{
			Name:  "AWS_DEFAULT_REGION",
			Value: config.Region,
		})
	}

	// Add custom STS endpoint for testing
	if config.STSEndpoint != "" {
		env = append(env, corev1.EnvVar{
			Name:  "AWS_ENDPOINT_URL_STS",
			Value: config.STSEndpoint,
		})
	}

	sidecar := corev1.Container{
		Name:            pkg.BrokerContainerName,
		Image:           config.BrokerImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args: []string{
			"--token-path=" + tokenMountPath + "/" + pkg.DefaultIRSATokenFile,
			"--credentials-path=" + credentialsPath + "/" + pkg.DefaultCredentialsFile,
		},
		Env: env,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      pkg.CredentialsVolumeName,
				MountPath: credentialsPath,
			},
			{
				Name:      pkg.BrokerTokenVolumeName,
				MountPath: tokenMountPath,
				ReadOnly:  true,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(pkg.DefaultBrokerCPURequest),
				corev1.ResourceMemory: resource.MustParse(pkg.DefaultBrokerMemoryRequest),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(pkg.DefaultBrokerCPULimit),
				corev1.ResourceMemory: resource.MustParse(pkg.DefaultBrokerMemoryLimit),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             boolPtr(true),
			ReadOnlyRootFilesystem:   boolPtr(true),
			AllowPrivilegeEscalation: boolPtr(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}

	var patches []PatchOperation

	// Add as regular container (sidecar)
	if pod.Spec.Containers == nil {
		patches = append(patches, PatchOperation{
			Op:    "add",
			Path:  "/spec/containers",
			Value: []corev1.Container{sidecar},
		})
	} else {
		patches = append(patches, PatchOperation{
			Op:    "add",
			Path:  "/spec/containers/-",
			Value: sidecar,
		})
	}

	return patches
}

func createContainerPatches(pod *corev1.Pod, config *PatchConfig, containersToSkip map[string]bool) ([]PatchOperation, bool) {
	var patches []PatchOperation
	changed := false

	credentialsPath := config.CredentialsPath
	if credentialsPath == "" {
		credentialsPath = pkg.DefaultCredentialsPath
	}

	credentialsFilePath := credentialsPath + "/" + pkg.DefaultCredentialsFile

	// Patch each container to add volume mount and env var
	for i, container := range pod.Spec.Containers {
		if containersToSkip[container.Name] {
			continue
		}
		// Skip the broker container itself
		if container.Name == pkg.BrokerContainerName {
			continue
		}

		// Check if AWS_SHARED_CREDENTIALS_FILE is already set
		hasCredentialsEnv := false
		for _, env := range container.Env {
			if env.Name == pkg.AwsEnvVarSharedCredentialsFile {
				hasCredentialsEnv = true
				break
			}
		}

		// Check if volume mount already exists
		hasMount := false
		for _, mount := range container.VolumeMounts {
			if mount.Name == pkg.CredentialsVolumeName {
				hasMount = true
				break
			}
		}

		// Add volume mount
		if !hasMount {
			if container.VolumeMounts == nil {
				patches = append(patches, PatchOperation{
					Op:   "add",
					Path: fmt.Sprintf("/spec/containers/%d/volumeMounts", i),
					Value: []corev1.VolumeMount{
						{
							Name:      pkg.CredentialsVolumeName,
							MountPath: credentialsPath,
							ReadOnly:  true,
						},
					},
				})
			} else {
				patches = append(patches, PatchOperation{
					Op:   "add",
					Path: fmt.Sprintf("/spec/containers/%d/volumeMounts/-", i),
					Value: corev1.VolumeMount{
						Name:      pkg.CredentialsVolumeName,
						MountPath: credentialsPath,
						ReadOnly:  true,
					},
				})
			}
			changed = true
		}

		// Add environment variable
		if !hasCredentialsEnv {
			if container.Env == nil {
				patches = append(patches, PatchOperation{
					Op:   "add",
					Path: fmt.Sprintf("/spec/containers/%d/env", i),
					Value: []corev1.EnvVar{
						{
							Name:  pkg.AwsEnvVarSharedCredentialsFile,
							Value: credentialsFilePath,
						},
					},
				})
			} else {
				patches = append(patches, PatchOperation{
					Op:   "add",
					Path: fmt.Sprintf("/spec/containers/%d/env/-", i),
					Value: corev1.EnvVar{
						Name:  pkg.AwsEnvVarSharedCredentialsFile,
						Value: credentialsFilePath,
					},
				})
			}
			changed = true
		}
	}

	// Also patch init containers
	for i, container := range pod.Spec.InitContainers {
		if containersToSkip[container.Name] {
			continue
		}

		// Check if volume mount already exists
		hasMount := false
		for _, mount := range container.VolumeMounts {
			if mount.Name == pkg.CredentialsVolumeName {
				hasMount = true
				break
			}
		}

		// Check if AWS_SHARED_CREDENTIALS_FILE is already set
		hasCredentialsEnv := false
		for _, env := range container.Env {
			if env.Name == pkg.AwsEnvVarSharedCredentialsFile {
				hasCredentialsEnv = true
				break
			}
		}

		// Add volume mount
		if !hasMount {
			if container.VolumeMounts == nil {
				patches = append(patches, PatchOperation{
					Op:   "add",
					Path: fmt.Sprintf("/spec/initContainers/%d/volumeMounts", i),
					Value: []corev1.VolumeMount{
						{
							Name:      pkg.CredentialsVolumeName,
							MountPath: credentialsPath,
							ReadOnly:  true,
						},
					},
				})
			} else {
				patches = append(patches, PatchOperation{
					Op:   "add",
					Path: fmt.Sprintf("/spec/initContainers/%d/volumeMounts/-", i),
					Value: corev1.VolumeMount{
						Name:      pkg.CredentialsVolumeName,
						MountPath: credentialsPath,
						ReadOnly:  true,
					},
				})
			}
			changed = true
		}

		// Add environment variable
		if !hasCredentialsEnv {
			if container.Env == nil {
				patches = append(patches, PatchOperation{
					Op:   "add",
					Path: fmt.Sprintf("/spec/initContainers/%d/env", i),
					Value: []corev1.EnvVar{
						{
							Name:  pkg.AwsEnvVarSharedCredentialsFile,
							Value: credentialsFilePath,
						},
					},
				})
			} else {
				patches = append(patches, PatchOperation{
					Op:   "add",
					Path: fmt.Sprintf("/spec/initContainers/%d/env/-", i),
					Value: corev1.EnvVar{
						Name:  pkg.AwsEnvVarSharedCredentialsFile,
						Value: credentialsFilePath,
					},
				})
			}
			changed = true
		}
	}

	return patches, changed
}

func boolPtr(b bool) *bool {
	return &b
}
