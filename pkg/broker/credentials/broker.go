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

// Package credentials provides the credential broker that fetches and refreshes
// AWS credentials with session tags for ABAC (Attribute-Based Access Control).
package credentials

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"k8s.io/klog/v2"
)

const (
	// DefaultRefreshBuffer is how long before expiry we refresh credentials
	DefaultRefreshBuffer = 5 * time.Minute
	// DefaultSessionDuration is the default STS session duration
	DefaultSessionDuration = 1 * time.Hour
	// MinRefreshInterval prevents spinning if credentials have very short TTL
	MinRefreshInterval = 30 * time.Second
)

// Config holds the broker configuration
type Config struct {
	// TokenPath is the path to the IRSA web identity token
	TokenPath string
	// BaseRoleARN is the role to assume via AssumeRoleWithWebIdentity
	BaseRoleARN string
	// TargetRoleARN is the shared role to assume with session tags
	TargetRoleARN string
	// SessionTags are the tags to attach to the assumed role session
	SessionTags map[string]string
	// CredentialsPath is where to write the AWS credentials file
	CredentialsPath string
	// SessionName is the name for the STS session
	SessionName string
	// RefreshBuffer is how long before expiry to refresh
	RefreshBuffer time.Duration
	// SessionDuration for AssumeRole calls
	SessionDuration time.Duration
	// Region for STS calls (optional, uses SDK default if empty)
	Region string
}

// Credentials holds AWS credential data
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
}

// Broker handles the credential brokering lifecycle
type Broker struct {
	config Config
	writer *CredentialWriter
}

// New creates a new Broker with the given configuration
func New(cfg Config) (*Broker, error) {
	if cfg.RefreshBuffer == 0 {
		cfg.RefreshBuffer = DefaultRefreshBuffer
	}
	if cfg.SessionDuration == 0 {
		cfg.SessionDuration = DefaultSessionDuration
	}
	if cfg.SessionName == "" {
		cfg.SessionName = "eks-broker"
	}

	// Validate required fields
	if cfg.TokenPath == "" {
		return nil, fmt.Errorf("TokenPath is required")
	}
	if cfg.BaseRoleARN == "" {
		return nil, fmt.Errorf("BaseRoleARN is required")
	}
	if cfg.TargetRoleARN == "" {
		return nil, fmt.Errorf("TargetRoleARN is required")
	}
	if cfg.CredentialsPath == "" {
		return nil, fmt.Errorf("CredentialsPath is required")
	}

	writer := NewCredentialWriter(cfg.CredentialsPath)

	return &Broker{
		config: cfg,
		writer: writer,
	}, nil
}

// Run starts the credential broker loop. It blocks until the context is cancelled.
func (b *Broker) Run(ctx context.Context) error {
	klog.Info("Starting credential broker")

	// Initial credential fetch
	expiry, err := b.refreshCredentials(ctx)
	if err != nil {
		return fmt.Errorf("initial credential refresh failed: %w", err)
	}

	for {
		// Calculate sleep duration
		sleepDuration := time.Until(expiry) - b.config.RefreshBuffer
		if sleepDuration < MinRefreshInterval {
			sleepDuration = MinRefreshInterval
		}

		klog.V(2).Infof("Credentials valid until %s, sleeping for %s", expiry.Format(time.RFC3339), sleepDuration)

		select {
		case <-ctx.Done():
			klog.Info("Context cancelled, stopping broker")
			return ctx.Err()
		case <-time.After(sleepDuration):
			klog.V(1).Info("Refreshing credentials")
			newExpiry, err := b.refreshCredentials(ctx)
			if err != nil {
				klog.Errorf("Failed to refresh credentials: %v", err)
				// Use exponential backoff on failure
				time.Sleep(10 * time.Second)
				continue
			}
			expiry = newExpiry
		}
	}
}

// refreshCredentials performs the two-step credential refresh:
// 1. AssumeRoleWithWebIdentity to get base credentials
// 2. AssumeRole with session tags to get tagged credentials
func (b *Broker) refreshCredentials(ctx context.Context) (time.Time, error) {
	// Step 1: Read the web identity token
	token, err := os.ReadFile(b.config.TokenPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to read token from %s: %w", b.config.TokenPath, err)
	}
	tokenStr := string(token)

	klog.V(3).Infof("Read token from %s (%d bytes)", b.config.TokenPath, len(token))

	// Step 2: AssumeRoleWithWebIdentity to get base credentials
	baseCreds, err := b.assumeRoleWithWebIdentity(ctx, tokenStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("AssumeRoleWithWebIdentity failed: %w", err)
	}

	klog.V(2).Infof("Obtained base credentials from AssumeRoleWithWebIdentity (expires: %s)",
		baseCreds.Expiration.Format(time.RFC3339))

	// Step 3: AssumeRole with session tags using the base credentials
	taggedCreds, err := b.assumeRoleWithTags(ctx, baseCreds)
	if err != nil {
		return time.Time{}, fmt.Errorf("AssumeRole with tags failed: %w", err)
	}

	klog.V(2).Infof("Obtained tagged credentials (expires: %s)", taggedCreds.Expiration.Format(time.RFC3339))

	// Step 4: Write credentials to file
	if err := b.writer.Write(taggedCreds); err != nil {
		return time.Time{}, fmt.Errorf("failed to write credentials: %w", err)
	}

	klog.Infof("Successfully refreshed credentials (expires: %s)", taggedCreds.Expiration.Format(time.RFC3339))

	return taggedCreds.Expiration, nil
}

// assumeRoleWithWebIdentity exchanges the web identity token for base credentials
func (b *Broker) assumeRoleWithWebIdentity(ctx context.Context, token string) (*Credentials, error) {
	// Create a minimal AWS config without credentials (we're providing the token)
	opts := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	}
	if b.config.Region != "" {
		opts = append(opts, config.WithRegion(b.config.Region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := sts.NewFromConfig(cfg)

	input := &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(b.config.BaseRoleARN),
		RoleSessionName:  aws.String(b.config.SessionName + "-base"),
		WebIdentityToken: aws.String(token),
		DurationSeconds:  aws.Int32(int32(b.config.SessionDuration.Seconds())),
	}

	result, err := client.AssumeRoleWithWebIdentity(ctx, input)
	if err != nil {
		return nil, err
	}

	return &Credentials{
		AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(result.Credentials.SessionToken),
		Expiration:      aws.ToTime(result.Credentials.Expiration),
	}, nil
}

// assumeRoleWithTags uses the base credentials to assume the target role with session tags
func (b *Broker) assumeRoleWithTags(ctx context.Context, baseCreds *Credentials) (*Credentials, error) {
	// Create AWS config with the base credentials
	opts := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			baseCreds.AccessKeyID,
			baseCreds.SecretAccessKey,
			baseCreds.SessionToken,
		)),
	}
	if b.config.Region != "" {
		opts = append(opts, config.WithRegion(b.config.Region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := sts.NewFromConfig(cfg)

	// Build session tags
	var tags []types.Tag
	for key, value := range b.config.SessionTags {
		tags = append(tags, types.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}

	input := &sts.AssumeRoleInput{
		RoleArn:         aws.String(b.config.TargetRoleARN),
		RoleSessionName: aws.String(b.config.SessionName),
		DurationSeconds: aws.Int32(int32(b.config.SessionDuration.Seconds())),
		Tags:            tags,
	}

	result, err := client.AssumeRole(ctx, input)
	if err != nil {
		return nil, err
	}

	return &Credentials{
		AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(result.Credentials.SessionToken),
		Expiration:      aws.ToTime(result.Credentials.Expiration),
	}, nil
}
