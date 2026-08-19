/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package opinfo holds facts about the environment the operator runs in: the identity it
// authenticates as and the DNS domain of its cluster.
package opinfo

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// defaultClusterDomain is the cluster DNS domain assumed until resolution succeeds.
	defaultClusterDomain = "cluster.local"

	// clusterDomainEnv configures the cluster's DNS domain.
	clusterDomainEnv = "KUBERNETES_CLUSTER_DOMAIN"

	resolvConfPath = "/etc/resolv.conf"

	// kubelet writes the cluster domain into the resolver search list as svc.<domain>
	serviceSearchPrefix = "svc."

	// ServiceAccount usernames take the form system:serviceaccount:<namespace>:<name>
	serviceAccountUsernamePrefix = "system:serviceaccount:"
)

// Written once by Resolve, read-only thereafter.
var (
	clusterDomain = defaultClusterDomain

	serviceAccount types.NamespacedName
)

// ClusterDomain returns the DNS domain of the cluster the operator runs in. It is never empty.
func ClusterDomain() string { return clusterDomain }

// ServiceAccount returns the ServiceAccount the operator authenticates as.
func ServiceAccount() types.NamespacedName { return serviceAccount }

// Resolve determines the cluster domain and the operator's own identity.
func Resolve(ctx context.Context, c client.Client) error {
	logger := logf.FromContext(ctx)
	resolvedDomain, err := resolveClusterDomain(logger)
	if err != nil {
		return err
	}
	clusterDomain = resolvedDomain

	sa, err := selfServiceAccountName(ctx, c)
	if err != nil {
		return err
	}
	serviceAccount = sa
	logger.Info("Resolved the operator identity", "serviceAccount", sa.String())
	return nil
}

// selfServiceAccountName returns the name of the ServiceAccount the given client authenticates as.
func selfServiceAccountName(ctx context.Context, c client.Client) (types.NamespacedName, error) {
	var review *authenticationv1.SelfSubjectReview
	backoff := wait.Backoff{Duration: time.Second, Factor: 2, Jitter: 0.1, Steps: 4}
	err := retry.OnError(backoff, retriableReviewError, func() error {
		review = &authenticationv1.SelfSubjectReview{}
		return c.Create(ctx, review)
	})
	if err != nil {
		return types.NamespacedName{}, fmt.Errorf("failed to review the operator's own identity: %w", err)
	}
	username := review.Status.UserInfo.Username
	sa, isServiceAccount := strings.CutPrefix(username, serviceAccountUsernamePrefix)
	if !isServiceAccount {
		return types.NamespacedName{}, fmt.Errorf("identity %q is not a ServiceAccount", username)
	}
	namespace, name, separated := strings.Cut(sa, ":")
	if !separated || namespace == "" || name == "" {
		return types.NamespacedName{}, fmt.Errorf("cannot determine a ServiceAccount from identity %q", username)
	}
	return types.NamespacedName{Namespace: namespace, Name: name}, nil
}

// retriableReviewError reports whether the API server may answer the same review differently.
func retriableReviewError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// a cluster that does not serve SelfSubjectReview fails in the RESTMapper, without a status
	if meta.IsNoMatchError(err) {
		return false
	}
	return !apierrors.IsUnauthorized(err) && !apierrors.IsForbidden(err) && !apierrors.IsNotFound(err)
}

func resolveClusterDomain(logger logr.Logger) (string, error) {
	// a fully qualified value would fail validation
	configured := strings.TrimSuffix(strings.TrimSpace(os.Getenv(clusterDomainEnv)), ".")
	if configured != "" {
		logger.Info("Using the configured cluster domain", "clusterDomain", configured, "env", clusterDomainEnv)
		return validDomain(configured)
	}
	discovered, err := discoverClusterDomain(resolvConfPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve cluster domain from %s: %w", resolvConfPath, err)
	}
	if discovered == "" {
		return "", fmt.Errorf("found no %s entry in the %s search list. Set %s to the cluster's DNS domain",
			serviceSearchPrefix, resolvConfPath, clusterDomainEnv)
	}
	logger.Info("Discovered the cluster domain", "clusterDomain", discovered)
	return validDomain(discovered)
}

// validDomain rejects a domain that cannot be used in a DNS name or a certificate.
func validDomain(domain string) (string, error) {
	if errs := validation.IsDNS1123Subdomain(domain); len(errs) > 0 {
		return "", fmt.Errorf("cluster domain %q is invalid: %s", domain, strings.Join(errs, "; "))
	}
	return domain, nil
}

// discoverClusterDomain reads the cluster domain out of the resolver configuration.
func discoverClusterDomain(resolver string) (string, error) {
	resolvConf, err := os.Open(resolver)
	if err != nil {
		return "", err
	}
	defer resolvConf.Close()
	return domainFromResolvConf(resolvConf)
}

// domainFromResolvConf returns an empty domain when the search list holds no cluster entry.
func domainFromResolvConf(resolvConf io.Reader) (string, error) {
	lines := bufio.NewScanner(resolvConf)
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		if len(fields) < 2 || fields[0] != "search" {
			continue
		}
		if domain := domainFromSearchList(fields[1:]); domain != "" {
			return domain, nil
		}
	}
	return "", lines.Err()
}

func domainFromSearchList(entries []string) string {
	for _, entry := range entries {
		// svc.cluster.local leaves cluster.local, while ais.svc.cluster.local is skipped
		domain, isServiceSearch := strings.CutPrefix(strings.TrimSuffix(entry, "."), serviceSearchPrefix)
		if isServiceSearch && domain != "" {
			return domain
		}
	}
	return ""
}
