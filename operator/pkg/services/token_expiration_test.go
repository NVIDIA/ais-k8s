// Package services contains services for the operator to use when reconciling AIS
/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package services

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTokenExpirationBackwardCompatibility(t *testing.T) {
	tests := []struct {
		name          string
		tokenExpireAt time.Time
		shouldBeValid bool
		description   string
	}{
		{
			name:          "No expiration (zero time)",
			tokenExpireAt: time.Time{},
			shouldBeValid: true,
			description:   "Tokens without expiration should always be valid",
		},
		{
			name:          "Future expiration (10 minutes)",
			tokenExpireAt: time.Now().Add(10 * time.Minute),
			shouldBeValid: true,
			description:   "Tokens expiring in more than TokenExpiryBuffer should be valid",
		},
		{
			name:          "Expiring soon (3 minutes)",
			tokenExpireAt: time.Now().Add(3 * time.Minute),
			shouldBeValid: false,
			description:   "Tokens expiring in less than TokenExpiryBuffer should be invalid",
		},
		{
			name:          "Already expired",
			tokenExpireAt: time.Now().Add(-1 * time.Minute),
			shouldBeValid: false,
			description:   "Expired tokens should be invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create a client with the test expiration time
			client := &AIStoreClient{
				ctx:           ctx,
				params:        nil, // We'll skip the nil check for this test
				mode:          "",  // Empty mode matches default GetAPIMode() return value
				tlsCfg:        nil,
				tokenExpireAt: tt.tokenExpireAt,
			}

			// Create a minimal AIStore spec
			ais := &aisv1.AIStore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: aisv1.AIStoreSpec{
					// Default values (APIMode is nil, so GetAPIMode() returns "")
				},
			}

			// Set params to non-nil to test expiration logic
			testURL := "http://test:8080"
			client.params = buildBaseParams(testURL, "", nil)

			// Check validity
			isValid := client.HasValidBaseParams(ctx, ais, testURL)

			if isValid != tt.shouldBeValid {
				t.Errorf("%s: expected valid=%v, got valid=%v. %s",
					tt.name, tt.shouldBeValid, isValid, tt.description)
			}
		})
	}
}

func TestTokenInfoStructure(t *testing.T) {
	// Test that TokenInfo correctly handles both cases
	t.Run("With expiration", func(t *testing.T) {
		expiresAt := time.Now().Add(1 * time.Hour)
		tokenInfo := &TokenInfo{
			Token:     "test-token",
			ExpiresAt: expiresAt,
		}

		if tokenInfo.Token != "test-token" {
			t.Errorf("Expected token 'test-token', got '%s'", tokenInfo.Token)
		}

		if tokenInfo.ExpiresAt.IsZero() {
			t.Error("Expected non-zero expiration time")
		}
	})

	t.Run("Without expiration", func(t *testing.T) {
		tokenInfo := &TokenInfo{
			Token:     "test-token",
			ExpiresAt: time.Time{}, // Zero value
		}

		if tokenInfo.Token != "test-token" {
			t.Errorf("Expected token 'test-token', got '%s'", tokenInfo.Token)
		}

		if !tokenInfo.ExpiresAt.IsZero() {
			t.Error("Expected zero expiration time for tokens without expiration")
		}
	})
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "non-HTTP error",
			err:      errors.New("connection refused"),
			expected: false,
		},
		{
			name: "401 Unauthorized",
			err: &cmn.ErrHTTP{
				Status: http.StatusUnauthorized,
			},
			expected: true,
		},
		{
			name: "403 Forbidden",
			err: &cmn.ErrHTTP{
				Status: http.StatusForbidden,
			},
			expected: true,
		},
		{
			name: "404 Not Found",
			err: &cmn.ErrHTTP{
				Status: http.StatusNotFound,
			},
			expected: false,
		},
		{
			name: "503 Service Unavailable",
			err: &cmn.ErrHTTP{
				Status: http.StatusServiceUnavailable,
			},
			expected: false,
		},
		{
			name: "500 Internal Server Error",
			err: &cmn.ErrHTTP{
				Status: http.StatusInternalServerError,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAuthError(tt.err)
			if result != tt.expected {
				t.Errorf("IsAuthError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestAuthFailedInvalidatesClient(t *testing.T) {
	ctx := context.Background()
	testURL := "http://test:8080"

	ais := &aisv1.AIStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: aisv1.AIStoreSpec{},
	}

	t.Run("client valid before auth failure", func(t *testing.T) {
		client := &AIStoreClient{
			ctx:    ctx,
			params: buildBaseParams(testURL, "", nil),
			mode:   "",
		}

		if !client.HasValidBaseParams(ctx, ais, testURL) {
			t.Error("Expected client to be valid before any auth failure")
		}
	})

	t.Run("client invalid after auth failure", func(t *testing.T) {
		client := &AIStoreClient{
			ctx:    ctx,
			params: buildBaseParams(testURL, "", nil),
			mode:   "",
		}

		// Simulate receiving a 401 error
		authErr := &cmn.ErrHTTP{Status: http.StatusUnauthorized}
		client.checkAuthErr(authErr)

		if client.HasValidBaseParams(ctx, ais, testURL) {
			t.Error("Expected client to be invalid after 401 error")
		}
	})

	t.Run("non-auth error does not invalidate client", func(t *testing.T) {
		client := &AIStoreClient{
			ctx:    ctx,
			params: buildBaseParams(testURL, "", nil),
			mode:   "",
		}

		// Simulate receiving a 500 error
		serverErr := &cmn.ErrHTTP{Status: http.StatusInternalServerError}
		client.checkAuthErr(serverErr)

		if !client.HasValidBaseParams(ctx, ais, testURL) {
			t.Error("Expected client to remain valid after non-auth error")
		}
	})

	t.Run("nil error does not invalidate client", func(t *testing.T) {
		client := &AIStoreClient{
			ctx:    ctx,
			params: buildBaseParams(testURL, "", nil),
			mode:   "",
		}

		client.checkAuthErr(nil)

		if !client.HasValidBaseParams(ctx, ais, testURL) {
			t.Error("Expected client to remain valid after nil error")
		}
	})

	t.Run("403 Forbidden invalidates client", func(t *testing.T) {
		client := &AIStoreClient{
			ctx:    ctx,
			params: buildBaseParams(testURL, "", nil),
			mode:   "",
		}

		authErr := &cmn.ErrHTTP{Status: http.StatusForbidden}
		client.checkAuthErr(authErr)

		if client.HasValidBaseParams(ctx, ais, testURL) {
			t.Error("Expected client to be invalid after 403 error")
		}
	})
}
