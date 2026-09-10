/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
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

func TestRefreshMarginClampedToTokenValidity(t *testing.T) {
	tests := []struct {
		name              string
		obtainedAgo       time.Duration
		expiresIn         time.Duration
		unknownObtainedAt bool
		wantExpired       bool
	}{
		{
			name:        "freshly obtained token with validity equal to the buffer",
			obtainedAgo: 0,
			expiresIn:   TokenExpiryBuffer,
			wantExpired: false,
		},
		{
			name:        "same token past half its validity",
			obtainedAgo: 3 * time.Minute,
			expiresIn:   2 * time.Minute,
			wantExpired: true,
		},
		{
			name:        "freshly obtained token with validity below the buffer",
			obtainedAgo: 0,
			expiresIn:   time.Minute,
			wantExpired: false,
		},
		{
			name:        "long-lived token retains the full buffer",
			obtainedAgo: 56 * time.Minute,
			expiresIn:   4 * time.Minute,
			wantExpired: true,
		},
		{
			name:              "no recorded obtain time retains the full buffer",
			expiresIn:         4 * time.Minute,
			unknownObtainedAt: true,
			wantExpired:       true,
		},
		{
			name:              "no recorded obtain time outside the full buffer",
			expiresIn:         10 * time.Minute,
			unknownObtainedAt: true,
			wantExpired:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			client := &AIStoreClient{
				tokenExpireAt: now.Add(tt.expiresIn),
			}
			if !tt.unknownObtainedAt {
				client.tokenObtainedAt = now.Add(-tt.obtainedAgo)
			}

			if got := client.isTokenExpired(); got != tt.wantExpired {
				t.Errorf("isTokenExpired() = %v, want %v (obtained %s ago, expires in %s)",
					got, tt.wantExpired, tt.obtainedAgo, tt.expiresIn)
			}
		})
	}
}

func TestTokenRefreshReason(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		obtainedAt    time.Time
		tokenExpireAt time.Time
		rejected      bool
		wantReason    string
	}{
		{
			name:          "token with time left",
			obtainedAt:    now,
			tokenExpireAt: now.Add(time.Hour),
		},
		{
			name: "token without expiration",
		},
		{
			name:          "token rejected by AIS",
			obtainedAt:    now,
			tokenExpireAt: now.Add(time.Hour),
			rejected:      true,
			wantReason:    "rejectedByAIS",
		},
		{
			name:          "token past its expiration",
			obtainedAt:    now.Add(-time.Hour),
			tokenExpireAt: now.Add(-time.Minute),
			wantReason:    "expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &AIStoreClient{tokenObtainedAt: tt.obtainedAt, tokenExpireAt: tt.tokenExpireAt}
			client.tokenRejected.Store(tt.rejected)

			if got := client.tokenRefreshReason(); got != tt.wantReason {
				t.Errorf("tokenRefreshReason() = %q, want %q", got, tt.wantReason)
			}
		})
	}
}

func TestAuthStatusTracker(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "forbidden", status: http.StatusForbidden, wantRejected: true},
		{name: "server error", status: http.StatusInternalServerError},
		{name: "not found", status: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			client := NewAIStoreClient(context.Background(), server.URL, &TokenInfo{Token: "test-token"}, "", nil)
			_ = client.Health(false)

			if got := client.tokenRejected.Load(); got != tt.wantRejected {
				t.Errorf("tokenRejected = %v, want %v", got, tt.wantRejected)
			}
		})
	}
}
