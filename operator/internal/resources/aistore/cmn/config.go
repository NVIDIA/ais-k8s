/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aiscos "github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	jsoniter "github.com/json-iterator/go"
)

const (
	defaultRebalanceState       = true
	ConfigHashAnnotation        = "config.aistore.nvidia.com/hash"
	RestartConfigHashAnnotation = "config.aistore.nvidia.com/restart-hash"
	RestartConfigHashInitial    = ".initial"
)

// Map keys are sorted so that a given spec always renders byte-identical config.
var configJSON = jsoniter.Config{SortMapKeys: true, EscapeHTML: false}.Froze()

// GenerateGlobalConfig creates the initial config override to supply to an AIS daemon pod
//
//	This pulls configs from the AIS spec and includes cluster or state specific changes.
//	Note that the result can be out of sync with the actual spec depending on cluster state
func GenerateGlobalConfig(ais *aisv1.AIStore) (*aiscmn.ConfigToSet, error) {
	// Create initial configuration with changes that we do NOT want to update with spec, e.g. primary proxy
	conf := newInitialConfig(ais)
	// Apply changes from AIS spec considering current state
	configToSet, err := GenerateConfigToSet(ais)
	if err != nil {
		return nil, err
	}
	conf.Merge(configToSet)
	return conf, nil
}

func newInitialConfig(ais *aisv1.AIStore) *aiscmn.ConfigToSet {
	defaultURL := aisapc.Ptr(DefaultProxyURL(ais))
	discoveryURL := aisapc.Ptr(DiscoveryProxyURL(ais))
	conf := &aiscmn.ConfigToSet{
		Proxy: &aiscmn.ProxyConfToSet{
			PrimaryURL:   defaultURL,
			OriginalURL:  defaultURL,
			DiscoveryURL: discoveryURL,
		},
	}
	return conf
}

// GenerateConfigToSet determines the actual config we want to apply based on config overrides provided in spec
func GenerateConfigToSet(ais *aisv1.AIStore) (*aiscmn.ConfigToSet, error) {
	specConfig := &aisv1.ConfigToUpdate{}
	if ais.Spec.ConfigToUpdate != nil {
		// Deep copy to avoid modifying the spec itself
		specConfig = ais.Spec.ConfigToUpdate.DeepCopy()
	}
	if ais.HasTLSEnabled() {
		if specConfig.Net == nil {
			specConfig.Net = &aisv1.NetConfToUpdate{}
		}
		if specConfig.Net.HTTP == nil {
			specConfig.Net.HTTP = &aisv1.HTTPConfToUpdate{}
		}
		specConfig.Net.HTTP.Certificate = aisapc.Ptr(filepath.Join(certsDir, TLSCertFileName))
		specConfig.Net.HTTP.CertKey = aisapc.Ptr(filepath.Join(certsDir, TLSKeyFileName))
		specConfig.Net.HTTP.ClientCA = aisapc.Ptr(filepath.Join(certsDir, TLSCAFileName))
	}

	// Override rebalance if the cluster is not ready for it (starting up, scaling, upgrading)
	if ais.IsConditionTrue(aisv1.ConditionReadyRebalance) {
		// If not provided, reset to default
		if !specConfig.IsRebalanceEnabledSet() {
			specConfig.UpdateRebalanceEnabled(aisapc.Ptr(defaultRebalanceState))
		}
	} else {
		specConfig.UpdateRebalanceEnabled(aisapc.Ptr(false))
	}

	if ais.Spec.HasCloudBackend() {
		specConfig.ConfigureBackend(&ais.Spec)
	}

	buildSpecConfigAuth(ais, specConfig)

	return specConfig.Convert()
}

func buildSpecConfigAuth(ais *aisv1.AIStore, specConfig *aisv1.ConfigToUpdate) {
	if ais.Spec.AuthNSecretName != nil && !specConfig.HasOIDCIssuers() {
		specConfig.EnsureHMACSignature()
	}
	if ais.Spec.NodeJoinSecretName() != nil {
		specConfig.ConfigureNodeJoin(filepath.Join(nodeJoinDir, NodeJoinSecretFileName))
	}
	// AIStore is not configured to use auth in any way
	if specConfig.Auth == nil {
		return
	}
	// Build OIDC issuer CA path from constants if ConfigMap is specified
	if ais.Spec.IssuerCAConfigMap != nil {
		specConfig.ConfigureOIDCIssuer(filepath.Join(OIDCCAMountPath, OIDCCAFileName))
	}
}

// configWithAuth marshals a ConfigToSet with its auth section replaced by pre-rendered JSON.
// The outer field shadows the embedded one, so auth is emitted verbatim.
type configWithAuth struct {
	*aiscmn.ConfigToSet
	Auth jsoniter.RawMessage `json:"auth,omitempty"`
}

// MarshalGlobalConfig serializes conf into the JSON form AIS consumes.
func MarshalGlobalConfig(ais *aisv1.AIStore, conf *aiscmn.ConfigToSet) ([]byte, error) {
	spec := ais.Spec.ConfigToUpdate
	// If no auth to consider, return as marshaled by AIS
	if conf == nil || conf.Auth == nil {
		return configJSON.Marshal(conf)
	}
	// Marshal legacy options for compatibility with AIS releases before v5.0.0
	auth, err := useLegacyAuthConf(conf.Auth, spec)
	if err != nil {
		return nil, err
	}
	return configJSON.Marshal(&configWithAuth{ConfigToSet: conf, Auth: auth})
}

// useLegacyAuthConf renders conf under the auth option names AIS used before v5.0.0, keeping the
// new name for each option the spec set that way. Options it does not rename are passed through.
func useLegacyAuthConf(conf *aiscmn.AuthConfToSet, specConf *aisv1.ConfigToUpdate) (jsoniter.RawMessage, error) {
	data, err := configJSON.Marshal(conf)
	if err != nil {
		return nil, err
	}
	auth := map[string]jsoniter.RawMessage{}
	err = configJSON.Unmarshal(data, &auth)
	if err != nil {
		return nil, err
	}
	if !specConf.ClientAuthRequiredSet() {
		renameKey(auth, "client_auth_required", "enabled")
	}
	if !specConf.IntraClusterSet() {
		err = useLegacyClusterKey(auth, conf.IntraCluster)
		if err != nil {
			return nil, err
		}
	}
	return configJSON.Marshal(auth)
}

// legacyClusterKeyConf is the auth.cluster_key section AIS accepted before v5.0.0. Options with no
// counterpart there are dropped, since those releases reject fields they do not know.
type legacyClusterKeyConf struct {
	Enabled       *bool            `json:"enabled,omitempty"`
	TTL           *aiscos.Duration `json:"ttl,omitempty"`
	NonceWindow   *aiscos.Duration `json:"nonce_window,omitempty"`
	RotationGrace *aiscos.Duration `json:"rotation_grace,omitempty"`
}

// useLegacyClusterKey replaces the intra_cluster section of auth with the cluster_key section AIS
// used before v5.0.0.
func useLegacyClusterKey(auth map[string]jsoniter.RawMessage, intra *aiscmn.IntraClusterConfToSet) error {
	if intra == nil {
		return nil
	}
	clusterKey, err := configJSON.Marshal(&legacyClusterKeyConf{
		Enabled:       intra.RequestAuth,
		TTL:           intra.TTL,
		NonceWindow:   intra.NonceWindow,
		RotationGrace: intra.RotationGrace,
	})
	if err != nil {
		return err
	}
	delete(auth, "intra_cluster")
	auth["cluster_key"] = clusterKey
	return nil
}

func renameKey(m map[string]jsoniter.RawMessage, from, to string) {
	if v, ok := m[from]; ok {
		delete(m, from)
		m[to] = v
	}
}

// HashGlobalConfig hashes the config bytes as they are written to AIS.
func HashGlobalConfig(conf []byte) string {
	hash := sha256.Sum256(conf)
	return hex.EncodeToString(hash[:])
}

// HashRestartConfigs generates a hash of ONLY configs that should trigger cluster restart upon change
func HashRestartConfigs(c *aiscmn.ConfigToSet) (string, error) {
	checksum := sha256.Sum256([]byte{})
	if c.Net != nil && c.Net.HTTP != nil {
		confNetHTTP, err := configJSON.Marshal(*c.Net.HTTP)
		if err != nil {
			return "", err
		}
		checksum = sha256.Sum256(confNetHTTP)
	}
	return hex.EncodeToString(checksum[:]), nil
}
