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

// Package main implements the credential broker sidecar that fetches
// and refreshes AWS credentials with session tags for ABAC.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/amazon-eks-pod-identity-webhook/pkg/broker/credentials"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

var (
	version = "dev"
)

func main() {
	// Credential broker flags
	tokenPath := flag.String("token-path", "/var/run/secrets/eks.amazonaws.com/serviceaccount/token",
		"Path to the IRSA web identity token")
	baseRoleARN := flag.String("base-role-arn", "",
		"ARN of the base role to assume via AssumeRoleWithWebIdentity")
	targetRoleARN := flag.String("target-role-arn", "",
		"ARN of the shared target role to assume with session tags")
	credentialsPath := flag.String("credentials-path", "/var/run/aws-credentials/credentials",
		"Path to write the AWS credentials file")
	sessionName := flag.String("session-name", "",
		"Name for the STS session (defaults to pod name or 'eks-broker')")
	sessionTags := flag.String("session-tags", "",
		"Comma-separated key=value pairs for session tags (e.g., 'project-id=abc,namespace=xyz')")
	region := flag.String("region", "",
		"AWS region for STS calls (uses SDK default if empty)")
	refreshBuffer := flag.Duration("refresh-buffer", 5*time.Minute,
		"How long before credential expiry to refresh")
	sessionDuration := flag.Duration("session-duration", 1*time.Hour,
		"Duration for STS sessions")

	// Health/metrics server
	metricsPort := flag.Int("metrics-port", 8080, "Port for health and metrics endpoints")

	// Version flag
	showVersion := flag.Bool("version", false, "Show version and exit")

	klog.InitFlags(nil)
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	// Build session name from pod name if not specified
	if *sessionName == "" {
		if podName := os.Getenv("POD_NAME"); podName != "" {
			*sessionName = podName
		} else {
			*sessionName = "eks-broker"
		}
	}

	// Parse session tags
	tags := make(map[string]string)

	// First, check for environment variable-based tags
	// These are injected by the webhook and take the form BROKER_TAG_<KEY>=<value>
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "BROKER_TAG_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				tagKey := strings.TrimPrefix(parts[0], "BROKER_TAG_")
				tagKey = strings.ToLower(strings.ReplaceAll(tagKey, "_", "-"))
				tags[tagKey] = parts[1]
			}
		}
	}

	// Parse command-line session tags (override env vars)
	if *sessionTags != "" {
		for _, pair := range strings.Split(*sessionTags, ",") {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				tags[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}

	// Validate required flags
	if *baseRoleARN == "" {
		// Check environment variable
		if envVal := os.Getenv("BROKER_BASE_ROLE_ARN"); envVal != "" {
			*baseRoleARN = envVal
		} else {
			klog.Fatal("--base-role-arn is required")
		}
	}
	if *targetRoleARN == "" {
		// Check environment variable
		if envVal := os.Getenv("BROKER_TARGET_ROLE_ARN"); envVal != "" {
			*targetRoleARN = envVal
		} else {
			klog.Fatal("--target-role-arn is required")
		}
	}

	if len(tags) == 0 {
		klog.Warning("No session tags specified. Credentials will not have ABAC tags.")
	}

	klog.Infof("Starting credential broker %s", version)
	klog.Infof("Token path: %s", *tokenPath)
	klog.Infof("Base role ARN: %s", *baseRoleARN)
	klog.Infof("Target role ARN: %s", *targetRoleARN)
	klog.Infof("Credentials path: %s", *credentialsPath)
	klog.Infof("Session tags: %v", tags)

	// Create broker
	cfg := credentials.Config{
		TokenPath:       *tokenPath,
		BaseRoleARN:     *baseRoleARN,
		TargetRoleARN:   *targetRoleARN,
		SessionTags:     tags,
		CredentialsPath: *credentialsPath,
		SessionName:     *sessionName,
		RefreshBuffer:   *refreshBuffer,
		SessionDuration: *sessionDuration,
		Region:          *region,
	}

	b, err := credentials.New(cfg)
	if err != nil {
		klog.Fatalf("Failed to create broker: %v", err)
	}

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		klog.Infof("Received signal %s, shutting down", sig)
		cancel()
	}()

	// Start health/metrics server
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
		mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
			// Check if credentials file exists and is recent
			info, err := os.Stat(*credentialsPath)
			if err != nil {
				http.Error(w, "credentials file not found", http.StatusServiceUnavailable)
				return
			}
			// Consider ready if file was modified in the last hour
			if time.Since(info.ModTime()) > time.Hour {
				http.Error(w, "credentials file is stale", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
		mux.Handle("/metrics", promhttp.Handler())

		addr := fmt.Sprintf(":%d", *metricsPort)
		klog.Infof("Starting health/metrics server on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
			klog.Errorf("Health server error: %v", err)
		}
	}()

	// Run broker
	if err := b.Run(ctx); err != nil && err != context.Canceled {
		klog.Fatalf("Broker failed: %v", err)
	}

	klog.Info("Broker stopped")
}
