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
)

func TestIsTokenExpired(t *testing.T) {
	tests := []struct {
		name              string
		obtainedAgo       time.Duration
		expiresIn         time.Duration
		noExpiration      bool
		unknownObtainedAt bool
		wantExpired       bool
	}{
		{
			name:         "token without expiration never expires",
			noExpiration: true,
			wantExpired:  false,
		},
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
			name:        "long-lived token retains the full buffer",
			obtainedAgo: 56 * time.Minute,
			expiresIn:   4 * time.Minute,
			wantExpired: true,
		},
		{
			name:        "token past its expiration",
			obtainedAgo: time.Hour,
			expiresIn:   -time.Minute,
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
			client := &AIStoreClient{}
			if !tt.noExpiration {
				client.tokenInfo.ExpiresAt = now.Add(tt.expiresIn)
			}
			if !tt.unknownObtainedAt {
				client.tokenInfo.ObtainedAt = now.Add(-tt.obtainedAgo)
			}

			if got := client.isTokenExpired(); got != tt.wantExpired {
				t.Errorf("isTokenExpired() = %v, want %v (obtained %s ago, expires in %s)",
					got, tt.wantExpired, tt.obtainedAgo, tt.expiresIn)
			}
		})
	}
}

func TestSetTokenInstallsTokenInfo(t *testing.T) {
	const testURL = "http://test:8080"
	ctx := context.Background()
	obtainedAt := time.Now()
	first := &TokenInfo{
		Token:      "token-a",
		ObtainedAt: obtainedAt,
		ExpiresAt:  obtainedAt.Add(time.Hour),
		ProfileGen: "prod-auth@1",
	}

	client := NewAIStoreClient(ctx, testURL, first, "", nil)
	assertToken(t, client, first)

	second := &TokenInfo{
		Token:      "token-b",
		ObtainedAt: obtainedAt,
		ExpiresAt:  obtainedAt.Add(2 * time.Hour),
		ProfileGen: "prod-auth@2",
	}
	client.tokenRejected.Store(true)
	client.setToken(second)
	assertToken(t, client, second)
	if client.tokenRejected.Load() {
		t.Error("tokenRejected = true after installing a new token, want false")
	}

	client.setToken(nil)
	assertToken(t, client, &TokenInfo{})

	assertToken(t, NewAIStoreClient(ctx, testURL, nil, "", nil), &TokenInfo{})
}

// assertToken checks that the client records want and presents its token to the AIS API.
func assertToken(t *testing.T, client *AIStoreClient, want *TokenInfo) {
	t.Helper()
	if client.tokenInfo != *want {
		t.Errorf("tokenInfo = %+v, want %+v", client.tokenInfo, *want)
	}
	if client.params.Token != want.Token {
		t.Errorf("params.Token = %q, want %q", client.params.Token, want.Token)
	}
}

func TestTokenRefreshReason(t *testing.T) {
	const profileGen = "prod-auth@1"
	now := time.Now()

	tests := []struct {
		name       string
		tokenInfo  TokenInfo
		profileGen string
		rejected   bool
		wantReason string
	}{
		{
			name:       "token from the current profile with time left",
			tokenInfo:  TokenInfo{ObtainedAt: now, ExpiresAt: now.Add(time.Hour), ProfileGen: profileGen},
			profileGen: profileGen,
		},
		{
			name:       "client of a cluster requesting no auth",
			profileGen: "",
		},
		{
			name:       "token from a superseded profile",
			tokenInfo:  TokenInfo{ObtainedAt: now, ExpiresAt: now.Add(time.Hour), ProfileGen: "prod-auth@0"},
			profileGen: profileGen,
			wantReason: "authProfileChanged",
		},
		{
			name:       "client holding no token of the cluster profile",
			profileGen: profileGen,
			wantReason: "noTokenForProfile",
		},
		{
			name:       "token of a profile the cluster no longer references",
			tokenInfo:  TokenInfo{ObtainedAt: now, ExpiresAt: now.Add(time.Hour), ProfileGen: profileGen},
			profileGen: "",
			wantReason: "authProfileRemoved",
		},
		{
			name:       "token rejected by AIS",
			tokenInfo:  TokenInfo{ObtainedAt: now, ExpiresAt: now.Add(time.Hour), ProfileGen: profileGen},
			profileGen: profileGen,
			rejected:   true,
			wantReason: "rejectedByAIS",
		},
		{
			name:       "token past its expiration",
			tokenInfo:  TokenInfo{ObtainedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute), ProfileGen: profileGen},
			profileGen: profileGen,
			wantReason: "expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &AIStoreClient{tokenInfo: tt.tokenInfo}
			client.tokenRejected.Store(tt.rejected)

			if got := client.tokenRefreshReason(tt.profileGen); got != tt.wantReason {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			client := NewAIStoreClient(context.Background(), server.URL, &TokenInfo{Token: "token-a"}, "", nil)
			_ = client.Health(false)

			if got := client.tokenRejected.Load(); got != tt.wantRejected {
				t.Errorf("tokenRejected = %v, want %v", got, tt.wantRejected)
			}
		})
	}
}
