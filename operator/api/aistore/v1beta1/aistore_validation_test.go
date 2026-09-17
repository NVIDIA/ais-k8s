/*
 * Copyright (c) 2025-2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1beta1

import (
	"context"
	"testing"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestUsesStateEmptyDir(t *testing.T) {
	tests := []struct {
		name         string
		stateStorage *StateStorage
		expected     bool
	}{
		{"unset state storage returns false", nil, false},
		{"other mode returns false", &StateStorage{HostPath: &StateHostPathConfig{Prefix: "/mnt"}}, false},
		{"emptyDir returns true", &StateStorage{EmptyDir: &StateEmptyDirConfig{}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ais := &AIStore{}
			ais.Spec.StateStorage = tt.stateStorage
			Expect(ais.Spec.UsesStateEmptyDir()).To(Equal(tt.expected))
		})
	}
}

func TestStateStorageAccessors(t *testing.T) {
	tests := []struct {
		name           string
		stateStorage   *StateStorage
		hostpathPrefix *string
		storageClass   *string
		wantHostPath   *string
		wantClass      *string
	}{
		{
			name:         "stateStorage hostPath",
			stateStorage: &StateStorage{HostPath: &StateHostPathConfig{Prefix: "/mnt"}},
			wantHostPath: aisapc.Ptr("/mnt"),
		},
		{
			name:         "stateStorage pvc",
			stateStorage: &StateStorage{PVC: &StatePVCConfig{StorageClass: "my-sc"}},
			wantClass:    aisapc.Ptr("my-sc"),
		},
		{
			name:         "stateStorage emptyDir resolves to neither",
			stateStorage: &StateStorage{EmptyDir: &StateEmptyDirConfig{}},
		},
		{
			name:           "deprecated hostpathPrefix",
			hostpathPrefix: aisapc.Ptr("/mnt"),
			wantHostPath:   aisapc.Ptr("/mnt"),
		},
		{
			name:         "deprecated stateStorageClass",
			storageClass: aisapc.Ptr("my-sc"),
			wantClass:    aisapc.Ptr("my-sc"),
		},
		{
			name:           "deprecated stateStorageClass wins over hostpathPrefix",
			hostpathPrefix: aisapc.Ptr("/mnt"),
			storageClass:   aisapc.Ptr("my-sc"),
			wantClass:      aisapc.Ptr("my-sc"),
		},
		{
			name:           "stateStorage wins over deprecated options",
			stateStorage:   &StateStorage{EmptyDir: &StateEmptyDirConfig{}},
			hostpathPrefix: aisapc.Ptr("/mnt"),
			storageClass:   aisapc.Ptr("my-sc"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			spec := AIStoreSpec{
				StateStorage:      tt.stateStorage,
				HostpathPrefix:    tt.hostpathPrefix,
				StateStorageClass: tt.storageClass,
			}
			Expect(spec.StateStorageHostPathPrefix()).To(Equal(tt.wantHostPath))
			Expect(spec.StateStoragePVCStorageClass()).To(Equal(tt.wantClass))
		})
	}
}

func TestValidateStateStorage(t *testing.T) {
	tests := []struct {
		name           string
		stateStorage   *StateStorage
		hostpathPrefix *string
		storageClass   *string
		wantErr        bool
	}{
		{
			name:         "only emptyDir is valid",
			stateStorage: &StateStorage{EmptyDir: &StateEmptyDirConfig{}},
		},
		{
			name:         "only hostPath is valid",
			stateStorage: &StateStorage{HostPath: &StateHostPathConfig{Prefix: "/mnt"}},
		},
		{
			name:         "only pvc is valid",
			stateStorage: &StateStorage{PVC: &StatePVCConfig{StorageClass: "my-sc"}},
		},
		{
			name:         "emptyDir and hostPath errors",
			stateStorage: &StateStorage{EmptyDir: &StateEmptyDirConfig{}, HostPath: &StateHostPathConfig{Prefix: "/mnt"}},
			wantErr:      true,
		},
		{
			name:         "emptyDir and pvc errors",
			stateStorage: &StateStorage{EmptyDir: &StateEmptyDirConfig{}, PVC: &StatePVCConfig{StorageClass: "my-sc"}},
			wantErr:      true,
		},
		{
			name:         "hostPath and pvc errors",
			stateStorage: &StateStorage{HostPath: &StateHostPathConfig{Prefix: "/mnt"}, PVC: &StatePVCConfig{StorageClass: "my-sc"}},
			wantErr:      true,
		},
		{
			name:         "no mode set errors",
			stateStorage: &StateStorage{},
			wantErr:      true,
		},
		{
			name:           "deprecated hostpathPrefix alone is valid",
			hostpathPrefix: aisapc.Ptr("/mnt"),
		},
		{
			name:         "deprecated stateStorageClass alone is valid",
			storageClass: aisapc.Ptr("my-sc"),
		},
		{
			name:           "both deprecated options together are valid",
			hostpathPrefix: aisapc.Ptr("/mnt"),
			storageClass:   aisapc.Ptr("my-sc"),
		},
		{
			name:    "no state storage at all errors",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ais := &AIStore{}
			ais.Spec.StateStorage = tt.stateStorage
			ais.Spec.HostpathPrefix = tt.hostpathPrefix
			ais.Spec.StateStorageClass = tt.storageClass
			warns, err := ais.validateStateStorage()
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			Expect(warns).To(BeEmpty())
		})
	}
}

func TestValidateShutdownWithEmptyDir(t *testing.T) {
	tests := []struct {
		name            string
		stateStorage    *StateStorage
		shutdownCluster *bool
		wantErr         bool
	}{
		{
			name:            "emptyDir with shutdown enabled errors",
			stateStorage:    &StateStorage{EmptyDir: &StateEmptyDirConfig{}},
			shutdownCluster: aisapc.Ptr(true),
			wantErr:         true,
		},
		{
			name:            "emptyDir with shutdown disabled is valid",
			stateStorage:    &StateStorage{EmptyDir: &StateEmptyDirConfig{}},
			shutdownCluster: aisapc.Ptr(false),
		},
		{
			name:         "emptyDir with shutdown nil is valid",
			stateStorage: &StateStorage{EmptyDir: &StateEmptyDirConfig{}},
		},
		{
			name:            "hostPath with shutdown enabled is valid",
			stateStorage:    &StateStorage{HostPath: &StateHostPathConfig{Prefix: "/mnt"}},
			shutdownCluster: aisapc.Ptr(true),
		},
		{
			name:            "pvc with shutdown enabled is valid",
			stateStorage:    &StateStorage{PVC: &StatePVCConfig{StorageClass: "my-sc"}},
			shutdownCluster: aisapc.Ptr(true),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ais := &AIStore{}
			ais.Spec.StateStorage = tt.stateStorage
			ais.Spec.ShutdownCluster = tt.shutdownCluster
			_, err := ais.validateShutdownWithEmptyDir()
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidateExternalAccess(t *testing.T) {
	lbPort := func(port int32) *ExternalAccessSpec {
		return &ExternalAccessSpec{LoadBalancer: &LoadBalancerSpec{Port: aisapc.Ptr(port)}}
	}
	tests := []struct {
		name   string
		proxy  *ExternalAccessSpec
		target *ExternalAccessSpec
		// wantErrMsgs are substrings the rejection must name
		wantErrMsgs []string
	}{
		{name: "no external access"},
		{name: "target external access without a LoadBalancer", target: &ExternalAccessSpec{}},
		{name: "target LoadBalancer without a port", target: &ExternalAccessSpec{LoadBalancer: &LoadBalancerSpec{}}},
		{name: "proxy LoadBalancer port", proxy: lbPort(443)},
		{
			name:        "target LoadBalancer port",
			target:      lbPort(443),
			wantErrMsgs: []string{"spec.targetSpec.externalAccess.loadBalancer.port", "443", "spec.targetSpec.portPublic"},
		},
		{
			name:        "target LoadBalancer port alongside a proxy port",
			proxy:       lbPort(443),
			target:      lbPort(8443),
			wantErrMsgs: []string{"spec.targetSpec.externalAccess.loadBalancer.port", "8443"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			g := NewWithT(subT)
			ais := &AIStore{}
			ais.Spec.ProxySpec.ExternalAccess = tt.proxy
			ais.Spec.TargetSpec.ExternalAccess = tt.target
			warnings, err := ais.validateExternalAccess()
			g.Expect(warnings).To(BeEmpty())
			if len(tt.wantErrMsgs) == 0 {
				g.Expect(err).NotTo(HaveOccurred())
				return
			}
			for _, msg := range tt.wantErrMsgs {
				g.Expect(err).To(MatchError(ContainSubstring(msg)))
			}
		})
	}
}

func TestValidateTLSCertPaths(t *testing.T) {
	tests := []struct {
		name    string
		tls     bool
		http    *HTTPConfToUpdate
		wantErr bool
	}{
		{
			name: "tls without configToUpdate is valid",
			tls:  true,
		},
		{
			name: "tls with non-path http fields is valid",
			tls:  true,
			http: &HTTPConfToUpdate{UseHTTPS: aisapc.Ptr(true), SkipVerifyCrt: aisapc.Ptr(false)},
		},
		{
			name: "cert paths without tls are valid",
			http: &HTTPConfToUpdate{
				Certificate: aisapc.Ptr("/etc/ais/tls.crt"),
				CertKey:     aisapc.Ptr("/etc/ais/tls.key"),
				ClientCA:    aisapc.Ptr("/etc/ais/ca.crt"),
			},
		},
		{
			name:    "tls with server_crt errors",
			tls:     true,
			http:    &HTTPConfToUpdate{Certificate: aisapc.Ptr("/etc/ais/tls.crt")},
			wantErr: true,
		},
		{
			name:    "tls with server_key errors",
			tls:     true,
			http:    &HTTPConfToUpdate{CertKey: aisapc.Ptr("/etc/ais/tls.key")},
			wantErr: true,
		},
		{
			name:    "tls with client_ca_tls errors",
			tls:     true,
			http:    &HTTPConfToUpdate{ClientCA: aisapc.Ptr("/etc/ais/ca.crt")},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ais := &AIStore{}
			if tt.tls {
				ais.Spec.TLS = &TLSSpec{SecretName: aisapc.Ptr("tls-certs")}
			}
			if tt.http != nil {
				ais.Spec.ConfigToUpdate = &ConfigToUpdate{Net: &NetConfToUpdate{HTTP: tt.http}}
			}
			_, err := ais.validateTLSCertPaths()
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func allPubCertPaths() *TLSConfToUpdate {
	return &TLSConfToUpdate{
		Certificate: aisapc.Ptr("/etc/ais/pub.crt"),
		CertKey:     aisapc.Ptr("/etc/ais/pub.key"),
		ClientCA:    aisapc.Ptr("/etc/ais/pub-ca.crt"),
	}
}

func TestValidatePublicTLSCertPaths(t *testing.T) {
	tests := []struct {
		name   string
		public bool
		pub    *TLSConfToUpdate
		// wantErrMsg is a substring the rejection must name
		wantErrMsg string
	}{
		{
			name:   "public tls without pub config is valid",
			public: true,
		},
		{
			name:   "public tls with client_auth_tls is valid",
			public: true,
			pub:    &TLSConfToUpdate{ClientAuthTLS: aisapc.Ptr(4)},
		},
		{
			name: "pub cert paths without public tls are valid",
			pub:  allPubCertPaths(),
		},
		{
			name:       "public tls with pub cert paths errors",
			public:     true,
			pub:        allPubCertPaths(),
			wantErrMsg: "configToUpdate.net.http.pub.[server_crt,server_key,client_ca_tls]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			g := NewWithT(subT)
			ais := &AIStore{}
			ais.Spec.TLS = &TLSSpec{SecretName: aisapc.Ptr("tls-certs")}
			if tt.public {
				ais.Spec.TLS.Public = &PublicTLSSpec{SecretName: aisapc.Ptr("pub-tls-certs")}
			}
			if tt.pub != nil {
				ais.Spec.ConfigToUpdate = &ConfigToUpdate{
					Net: &NetConfToUpdate{HTTP: &HTTPConfToUpdate{Pub: tt.pub}},
				}
			}
			_, err := ais.validatePublicTLSCertPaths()
			if tt.wantErrMsg == "" {
				g.Expect(err).ToNot(HaveOccurred())
				return
			}
			g.Expect(err).To(MatchError(ContainSubstring(tt.wantErrMsg)))
		})
	}
}

func TestValidateSpecRejectsPublicTLSCertPaths(t *testing.T) {
	g := NewWithT(t)
	ais := &AIStore{
		Spec: AIStoreSpec{
			Size:         aisapc.Ptr[int32](1),
			StateStorage: &StateStorage{HostPath: &StateHostPathConfig{Prefix: "/mnt"}},
			TLS: &TLSSpec{
				SecretName: aisapc.Ptr("tls-certs"),
				Public:     &PublicTLSSpec{SecretName: aisapc.Ptr("pub-tls-certs")},
			},
			ConfigToUpdate: &ConfigToUpdate{
				Net: &NetConfToUpdate{HTTP: &HTTPConfToUpdate{Pub: allPubCertPaths()}},
			},
		},
	}
	_, err := ais.ValidateSpec(context.Background())
	g.Expect(err).To(MatchError(ContainSubstring("configToUpdate.net.http.pub.")))
}

func newSafeDecommAIS(mode ScaleDownMode, rebalance *bool) *AIStore {
	ais := &AIStore{}
	ais.Spec.TargetSpec.ScaleDownMode = mode
	if rebalance != nil {
		ais.Spec.ConfigToUpdate = &ConfigToUpdate{}
		ais.Spec.ConfigToUpdate.UpdateRebalanceEnabled(rebalance)
	}
	return ais
}

func TestValidateSafeDecommission(t *testing.T) {
	tests := []struct {
		name      string
		mode      ScaleDownMode
		rebalance *bool
		wantWarn  bool
	}{
		{"warns when rebalance is disabled", ScaleDownModeSafeDecommission, aisapc.Ptr(false), true},
		{"no warning when rebalance is enabled", ScaleDownModeSafeDecommission, aisapc.Ptr(true), false},
		{"no warning when rebalance is unset", ScaleDownModeSafeDecommission, nil, false},
		{"no warning for decommission mode", ScaleDownModeDecommission, aisapc.Ptr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			g := NewWithT(subT)
			warnings, err := newSafeDecommAIS(tt.mode, tt.rebalance).validateSafeDecommission()
			g.Expect(err).NotTo(HaveOccurred())
			if tt.wantWarn {
				g.Expect(warnings).NotTo(BeEmpty())
			} else {
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

func newAuthAIS(auth *AuthSpec, conf *ConfigToUpdate, authNSecret *string) *AIStore {
	ais := &AIStore{}
	ais.Spec.Auth = auth
	ais.Spec.ConfigToUpdate = conf
	ais.Spec.AuthNSecretName = authNSecret
	return ais
}

// TestValidateAuthConfig covers the spec and config combinations that cannot be applied together,
// including the OIDC options that coexist with an HMAC secret.
func TestValidateAuthConfig(t *testing.T) {
	var (
		secret  = aisapc.Ptr("hmac-secret")
		issuers = &ConfigToUpdate{Auth: &AuthConfToUpdate{
			OIDC: &OIDCConfToUpdate{AllowedIssuers: aisapc.Ptr([]string{"https://issuer.example.com"})},
		}}
		emptyIssuers = &ConfigToUpdate{Auth: &AuthConfToUpdate{
			OIDC: &OIDCConfToUpdate{AllowedIssuers: aisapc.Ptr([]string{})},
		}}
		issuerCA = &ConfigToUpdate{Auth: &AuthConfToUpdate{
			OIDC: &OIDCConfToUpdate{IssuerCA: aisapc.Ptr("/etc/ais/oidc-ca/ca.crt")},
		}}
	)
	tests := []struct {
		name        string
		conf        *ConfigToUpdate
		authNSecret *string
		// wantErrMsgs are substrings the rejection must name
		wantErrMsgs []string
	}{
		{name: "no auth configuration"},
		{name: "hmac secret without config", authNSecret: secret},
		{name: "hmac secret with auth enabled", conf: &ConfigToUpdate{Auth: &AuthConfToUpdate{Enabled: aisapc.Ptr(true)}}, authNSecret: secret},
		{name: "hmac secret with an OIDC issuer CA", conf: issuerCA, authNSecret: secret},
		{name: "hmac secret with empty allowed issuers", conf: emptyIssuers, authNSecret: secret},
		{name: "OIDC issuers without an hmac secret", conf: issuers},
		{
			name:        "hmac secret with OIDC issuers",
			conf:        issuers,
			authNSecret: secret,
			wantErrMsgs: []string{"spec.authNSecretName", "OIDC issuers"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			g := NewWithT(subT)
			warnings, err := newAuthAIS(nil, tt.conf, tt.authNSecret).validateAuthConfig()
			g.Expect(warnings).To(BeEmpty())
			if len(tt.wantErrMsgs) == 0 {
				g.Expect(err).NotTo(HaveOccurred())
				return
			}
			for _, msg := range tt.wantErrMsgs {
				g.Expect(err).To(MatchError(ContainSubstring(msg)))
			}
		})
	}
}

func TestValidateAuth(t *testing.T) {
	var (
		authEnabled  = &ConfigToUpdate{Auth: &AuthConfToUpdate{Enabled: aisapc.Ptr(true)}}
		authDisabled = &ConfigToUpdate{Auth: &AuthConfToUpdate{Enabled: aisapc.Ptr(false)}}
		authRequired = &ConfigToUpdate{Auth: &AuthConfToUpdate{ClientAuthRequired: aisapc.Ptr(true)}}
		profile      = &AuthSpec{ProfileRef: &AuthProfileRef{Name: "provider"}}
	)
	tests := []struct {
		name string
		auth *AuthSpec
		conf *ConfigToUpdate
		// wantErrMsgs are substrings the rejection must name
		wantErrMsgs []string
		wantWarn    bool
	}{
		{name: "no auth configuration"},
		{name: "auth section without enabled", conf: &ConfigToUpdate{Auth: &AuthConfToUpdate{}}},
		{name: "auth explicitly disabled", conf: authDisabled},
		{name: "profileRef alone", auth: profile, wantWarn: true},
		{name: "profileRef with auth disabled", auth: profile, conf: authDisabled, wantWarn: true},
		{name: "profileRef with auth section but no enabled", auth: profile, conf: &ConfigToUpdate{Auth: &AuthConfToUpdate{}}, wantWarn: true},
		{name: "profileRef with auth enabled", auth: profile, conf: authEnabled},
		{
			name:        "spec.auth without profileRef",
			auth:        &AuthSpec{},
			wantErrMsgs: []string{"spec.auth is empty", "spec.auth.profileRef"},
		},
		{
			name:        "spec.auth without profileRef while auth enabled",
			auth:        &AuthSpec{},
			conf:        authEnabled,
			wantErrMsgs: []string{"spec.auth is empty", "spec.auth.profileRef"},
		},
		{
			name:        "auth enabled without spec.auth",
			conf:        authEnabled,
			wantErrMsgs: []string{"authenticate client requests", "spec.auth.profileRef"},
		},
		// Keeps validateAuth honest about reading RequiresClientAuth rather than auth.enabled itself.
		{
			name:        "client auth required without spec.auth",
			conf:        authRequired,
			wantErrMsgs: []string{"authenticate client requests", "spec.auth.profileRef"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			g := NewWithT(subT)
			warnings, err := newAuthAIS(tt.auth, tt.conf, nil).validateAuth()
			if tt.wantWarn {
				g.Expect(warnings).To(HaveLen(1))
				g.Expect(warnings[0]).To(ContainSubstring("not configured to authenticate client requests"))
			} else {
				g.Expect(warnings).To(BeEmpty())
			}
			if len(tt.wantErrMsgs) == 0 {
				g.Expect(err).NotTo(HaveOccurred())
				return
			}
			for _, msg := range tt.wantErrMsgs {
				g.Expect(err).To(MatchError(ContainSubstring(msg)))
			}
		})
	}
}

func TestValidateDeprecatedAuthFields(t *testing.T) {
	tests := []struct {
		name     string
		conf     *ConfigToUpdate
		wantWarn []string
	}{
		{name: "no config"},
		{name: "no auth options", conf: &ConfigToUpdate{Auth: &AuthConfToUpdate{}}},
		{
			name:     "deprecated enabled",
			conf:     &ConfigToUpdate{Auth: &AuthConfToUpdate{Enabled: aisapc.Ptr(true)}},
			wantWarn: []string{"spec.configToUpdate.auth.enabled"},
		},
		{
			name: "deprecated cluster_key",
			conf: &ConfigToUpdate{Auth: &AuthConfToUpdate{
				ClusterKey: &ClusterKeyConfToUpdate{Enabled: aisapc.Ptr(true)},
			}},
			wantWarn: []string{"spec.configToUpdate.auth.cluster_key"},
		},
		{
			name: "replacements warn about nothing",
			conf: &ConfigToUpdate{Auth: &AuthConfToUpdate{
				ClientAuthRequired: aisapc.Ptr(true),
				IntraCluster:       &IntraClusterConfToUpdate{RequestAuth: aisapc.Ptr(true)},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			g := NewWithT(subT)
			ais := &AIStore{}
			ais.Spec.ConfigToUpdate = tt.conf
			warnings, err := ais.validateDeprecatedFields()
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(HaveLen(len(tt.wantWarn)))
			for _, msg := range tt.wantWarn {
				g.Expect(warnings).To(ContainElement(ContainSubstring(msg)))
			}
		})
	}
}

func TestAIStoreValidateSize(t *testing.T) {
	tests := []struct {
		name       string // description of this test case
		want       admission.Warnings
		wantErr    bool
		proxySize  *int32
		targetSize *int32
		size       *int32
	}{
		{
			"Proxy size is -1 thus proxy autoscaling is true",
			nil,
			false,
			aisapc.Ptr[int32](-1),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"target size is -1 thus target autoscaling is true",
			nil,
			false,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](-1),
			nil,
		},
		{
			" size is -1 thus autoscaling is true",
			nil,
			false,
			nil,
			nil,
			aisapc.Ptr[int32](-1),
		},
		{
			"autoscaling",
			nil,
			false,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](-1),
		},
		{
			"not autoscaling",
			nil,
			false,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"not autoscaling with just size",
			nil,
			false,
			nil,
			nil,
			aisapc.Ptr[int32](1),
		},
		{
			"invalid size",
			nil,
			true,
			nil,
			nil,
			aisapc.Ptr[int32](-2),
		},
		{
			"invalid target size",
			nil,
			true,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](-2),
			nil,
		},
		{
			"invalid proxy size",
			nil,
			true,
			aisapc.Ptr[int32](-2),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"invalid proxy size;0",
			nil,
			true,
			aisapc.Ptr[int32](0),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"invalid target size;0",
			nil,
			true,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](0),
			nil,
		},
		{
			"invalid target size",
			nil,
			true,
			aisapc.Ptr[int32](0),
			aisapc.Ptr[int32](0),
			aisapc.Ptr[int32](0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			var ais AIStore
			ais.Spec.ProxySpec.Size = tt.proxySize
			ais.Spec.TargetSpec.Size = tt.targetSize
			ais.Spec.Size = tt.size
			got, gotErr := ais.validateSize()
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("validateSize() failed: %v for test %s", gotErr, tt.name)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("validateSize() succeeded unexpectedly")
			}
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
