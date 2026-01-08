/*
Copyright 2020 Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License").
You may not use this file except in compliance with the License.
A copy of the License is located at

	http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed
on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
express or implied. See the License for the specific language governing
permissions and limitations under the License.
*/
package pkg

const (
	// 24hrs as that is max for EKS
	MaxTokenExpiration = int64(86400)
	// Default token expiration in seconds if none is defined, 22hrs
	DefaultTokenExpiration = int64(86400)
	// Used for the minimum jitter value when using the default token expiration
	DefaultMinTokenExpiration = int64(79200)
	// 10mins is min for kube-apiserver
	MinTokenExpiration = int64(600)

	// AWS SDK defined environment variables.
	AwsEnvVarContainerCredentialsFullUri     = "AWS_CONTAINER_CREDENTIALS_FULL_URI"
	AwsEnvVarContainerAuthorizationTokenFile = "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE"

	// Credential Broker (Session Tags Mode) constants
	// Default broker sidecar image
	DefaultBrokerImage = "ghcr.io/aws/eks-pod-identity-broker:latest"
	// Default path for shared AWS credentials file
	DefaultCredentialsPath = "/var/run/aws-credentials"
	// Default credentials filename
	DefaultCredentialsFile = "credentials"
	// Environment variable for shared credentials file path
	AwsEnvVarSharedCredentialsFile = "AWS_SHARED_CREDENTIALS_FILE"
	// Broker container name
	BrokerContainerName = "aws-credential-broker"
	// Credentials volume name
	CredentialsVolumeName = "aws-credentials"
	// IRSA token volume name for broker
	BrokerTokenVolumeName = "aws-iam-token"
	// Default IRSA token mount path
	DefaultIRSATokenPath = "/var/run/secrets/eks.amazonaws.com/serviceaccount"
	// Default IRSA token filename
	DefaultIRSATokenFile = "token"

	// Broker resource defaults
	DefaultBrokerCPURequest    = "10m"
	DefaultBrokerMemoryRequest = "32Mi"
	DefaultBrokerCPULimit      = "100m"
	DefaultBrokerMemoryLimit   = "64Mi"
)
