/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"testing"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
)

func TestSyncPublicURLEndpointChange(t *testing.T) {
	const (
		currentURL   = "http://10.0.0.1:51080"
		newURL       = "http://10.0.0.2:51080"
		newSecureURL = "https://10.0.0.2:51080"
	)
	publicMode := APIModePublic

	tests := []struct {
		name          string
		mode          *string
		useHTTPS      bool
		discoveredURL string
		wantValid     bool
		wantURL       string
	}{
		{
			name:          "public mode adopts the new endpoint",
			mode:          &publicMode,
			discoveredURL: newURL,
			wantValid:     true,
			wantURL:       newURL,
		},
		{
			name:          "public mode keeps the endpoint when the scheme changes",
			mode:          &publicMode,
			useHTTPS:      true,
			discoveredURL: newSecureURL,
			wantValid:     false,
			wantURL:       currentURL,
		},
		{
			name:          "other modes invalidate the client",
			discoveredURL: newURL,
			wantValid:     false,
			wantURL:       currentURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ais := &aisv1.AIStore{Spec: aisv1.AIStoreSpec{APIMode: tt.mode}}
			if tt.useHTTPS {
				ais.Spec.ConfigToUpdate = &aisv1.ConfigToUpdate{
					Net: &aisv1.NetConfToUpdate{
						HTTP: &aisv1.HTTPConfToUpdate{UseHTTPS: aisapc.Ptr(true)},
					},
				}
			}
			client := &AIStoreClient{
				ctx:    ctx,
				params: buildBaseParams(currentURL, "", nil),
				mode:   ais.GetAPIMode(),
			}

			client.syncPublicURL(ctx, tt.discoveredURL)
			if got := client.HasValidBaseParams(ctx, ais, tt.discoveredURL); got != tt.wantValid {
				t.Errorf("HasValidBaseParams() = %v, want %v", got, tt.wantValid)
			}
			if client.params.URL != tt.wantURL {
				t.Errorf("client URL = %q, want %q", client.params.URL, tt.wantURL)
			}
		})
	}
}
