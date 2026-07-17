/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestAIStoreAuthProfileValidateSpec(t *testing.T) {
	t.Run("accepts valid https profile", func(t *testing.T) {
		g := NewWithT(t)
		profile := &AIStoreAuthProfile{
			Spec: AIStoreAuthProfileSpec{
				ServiceURL: "https://auth-provider.ais.svc:52001",
			},
		}
		g.Expect(profile.ValidateSpec()).To(Succeed())
	})

	t.Run("accepts valid http profile", func(t *testing.T) {
		g := NewWithT(t)
		profile := &AIStoreAuthProfile{
			Spec: AIStoreAuthProfileSpec{
				ServiceURL: "http://auth-provider.ais.svc:52001",
			},
		}
		g.Expect(profile.ValidateSpec()).To(Succeed())
	})

	t.Run("rejects missing serviceURL", func(t *testing.T) {
		g := NewWithT(t)
		profile := &AIStoreAuthProfile{}
		g.Expect(profile.ValidateSpec()).To(MatchError(ContainSubstring("spec.serviceURL must include a host")))
	})

	t.Run("rejects serviceURL with path", func(t *testing.T) {
		g := NewWithT(t)
		profile := &AIStoreAuthProfile{
			Spec: AIStoreAuthProfileSpec{
				ServiceURL: "https://auth-provider.ais.svc:52001/token",
			},
		}
		g.Expect(profile.ValidateSpec()).To(MatchError(ContainSubstring("must not include a path")))
	})

	t.Run("rejects invalid scheme", func(t *testing.T) {
		g := NewWithT(t)
		profile := &AIStoreAuthProfile{
			Spec: AIStoreAuthProfileSpec{
				ServiceURL: "ftp://auth-provider.ais.svc",
			},
		}
		g.Expect(profile.ValidateSpec()).To(MatchError(ContainSubstring("scheme must be http or https")))
	})
}

func TestValidateTokenExchangeEndpoint(t *testing.T) {
	tests := []struct {
		name          string
		tokenExchange *AuthProfileTokenExchange
		wantErr       string
	}{
		{
			name: "accepts omitted token exchange",
		},
		{
			name:          "accepts absolute path",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "/token"},
		},
		{
			name:          "rejects empty endpoint",
			tokenExchange: &AuthProfileTokenExchange{},
			wantErr:       "must be an absolute path without query or fragment",
		},
		{
			name:          "rejects whitespace endpoint",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: " \t "},
			wantErr:       "spec.tokenExchange.endpoint is invalid",
		},
		{
			name:          "rejects URL",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "https://auth-provider.ais.svc/token"},
			wantErr:       "must be a path, not a URL",
		},
		{
			name:          "rejects relative path",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "token"},
			wantErr:       "must be an absolute path without query or fragment",
		},
		{
			name:          "rejects query",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "/token?audience=ais"},
			wantErr:       "must be an absolute path without query or fragment",
		},
		{
			name:          "rejects fragment",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "/token#exchange"},
			wantErr:       "must be an absolute path without query or fragment",
		},
		{
			name:          "rejects malformed endpoint",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "/token%"},
			wantErr:       "spec.tokenExchange.endpoint is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			profile := &AIStoreAuthProfile{
				Spec: AIStoreAuthProfileSpec{TokenExchange: tt.tokenExchange},
			}

			err := profile.validateTokenExchangeEndpoint()
			if tt.wantErr == "" {
				g.Expect(err).To(Succeed())
				return
			}
			g.Expect(err).To(MatchError(ContainSubstring(tt.wantErr)))
		})
	}
}
