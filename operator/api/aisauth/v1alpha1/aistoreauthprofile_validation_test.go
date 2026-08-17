/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"testing"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestAIStoreAuthProfileValidateSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    AIStoreAuthProfileSpec
		wantErr []string
	}{
		{
			name: "accepts valid https profile",
			spec: AIStoreAuthProfileSpec{
				ServiceURL:    "https://auth-provider.ais.svc:52001",
				TokenExchange: &AuthProfileTokenExchange{Endpoint: "/token"},
			},
		},
		{
			name: "accepts valid http profile",
			spec: AIStoreAuthProfileSpec{
				ServiceURL:    "http://auth-provider.ais.svc:52001",
				TokenExchange: &AuthProfileTokenExchange{Endpoint: "/token"},
			},
		},
		{
			name:    "rejects missing serviceURL",
			wantErr: []string{"spec.serviceURL", "must include a host"},
		},
		{
			name: "rejects serviceURL with path",
			spec: AIStoreAuthProfileSpec{
				ServiceURL:    "https://auth-provider.ais.svc:52001/token",
				TokenExchange: &AuthProfileTokenExchange{Endpoint: "/token"},
			},
			wantErr: []string{"must not include a path"},
		},
		{
			name: "accepts OAuth password-grant token endpoint",
			spec: AIStoreAuthProfileSpec{
				ServiceURL: "https://keycloak.example",
				UsernamePassword: &AuthProfileUsernamePassword{
					Secret: AuthProfileSecret{Name: "creds", Namespace: "ais"},
					LoginConf: &AuthProfileLoginConf{
						ClientID: "AIStore",
						Endpoint: "/realms/aistore/protocol/openid-connect/token",
					},
				},
			},
		},
		{
			name: "rejects OAuth token endpoint URL",
			spec: AIStoreAuthProfileSpec{
				ServiceURL: "https://keycloak.example",
				UsernamePassword: &AuthProfileUsernamePassword{
					Secret: AuthProfileSecret{Name: "creds", Namespace: "ais"},
					LoginConf: &AuthProfileLoginConf{
						ClientID: "AIStore",
						Endpoint: "https://keycloak.example/token",
					},
				},
			},
			wantErr: []string{"spec.usernamePassword.loginConf.endpoint", "must be a path, not a URL"},
		},
		{
			name: "rejects relative OAuth token endpoint",
			spec: AIStoreAuthProfileSpec{
				ServiceURL: "https://keycloak.example",
				UsernamePassword: &AuthProfileUsernamePassword{
					Secret: AuthProfileSecret{Name: "creds", Namespace: "ais"},
					LoginConf: &AuthProfileLoginConf{
						ClientID: "AIStore",
						Endpoint: "token",
					},
				},
			},
			wantErr: []string{"must be an absolute path without query or fragment"},
		},
		{
			name: "rejects invalid scheme",
			spec: AIStoreAuthProfileSpec{
				ServiceURL:    "ftp://auth-provider.ais.svc",
				TokenExchange: &AuthProfileTokenExchange{Endpoint: "/token"},
			},
			wantErr: []string{"scheme must be http or https"},
		},
		{
			name:    "rejects missing auth method",
			spec:    AIStoreAuthProfileSpec{ServiceURL: "https://auth-provider.ais.svc"},
			wantErr: []string{"exactly one of usernamePassword or tokenExchange"},
		},
		{
			name: "rejects multiple auth methods",
			spec: AIStoreAuthProfileSpec{
				ServiceURL:       "https://auth-provider.ais.svc",
				UsernamePassword: &AuthProfileUsernamePassword{},
				TokenExchange:    &AuthProfileTokenExchange{Endpoint: "/token"},
			},
			wantErr: []string{"exactly one of usernamePassword or tokenExchange"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			err := (&AIStoreAuthProfile{Spec: tt.spec}).ValidateSpec()
			if len(tt.wantErr) == 0 {
				g.Expect(err).To(Succeed())
				return
			}
			g.Expect(apierrors.IsInvalid(err)).To(BeTrue())
			for _, substr := range tt.wantErr {
				g.Expect(err).To(MatchError(ContainSubstring(substr)))
			}
		})
	}
}

func TestValidateTokenExchangeEndpoint(t *testing.T) {
	tests := []struct {
		name          string
		tokenExchange *AuthProfileTokenExchange
		wantErr       bool
		// wantMsg is only set where the message is ours rather than reported by url.Parse
		wantMsg string
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
			wantErr:       true,
			wantMsg:       "must be an absolute path without query or fragment",
		},
		{
			name:          "rejects whitespace endpoint",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: " \t "},
			wantErr:       true,
		},
		{
			name:          "rejects URL",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "https://auth-provider.ais.svc/token"},
			wantErr:       true,
			wantMsg:       "must be a path, not a URL",
		},
		{
			name:          "rejects relative path",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "token"},
			wantErr:       true,
			wantMsg:       "must be an absolute path without query or fragment",
		},
		{
			name:          "rejects query",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "/token?audience=ais"},
			wantErr:       true,
			wantMsg:       "must be an absolute path without query or fragment",
		},
		{
			name:          "rejects fragment",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "/token#exchange"},
			wantErr:       true,
			wantMsg:       "must be an absolute path without query or fragment",
		},
		{
			name:          "rejects malformed endpoint",
			tokenExchange: &AuthProfileTokenExchange{Endpoint: "/token%"},
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			profile := &AIStoreAuthProfile{
				Spec: AIStoreAuthProfileSpec{TokenExchange: tt.tokenExchange},
			}

			errs := profile.validateTokenExchangeEndpoint()
			if !tt.wantErr {
				g.Expect(errs).To(BeEmpty())
				return
			}
			g.Expect(errs).NotTo(BeEmpty())
			g.Expect(errs[0].Field).To(Equal("spec.tokenExchange.endpoint"))
			if tt.wantMsg != "" {
				g.Expect(errs.ToAggregate().Error()).To(ContainSubstring(tt.wantMsg))
			}
		})
	}
}
