/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */

package v1beta1

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aiscos "github.com/NVIDIA/aistore/cmn/cos"
)

const SigningKeyMethodHMAC = "HMAC"

// NOTE: `*ToUpdate` structures are duplicates of `*ToSet` structs from AIStore main repository.
// For custom types used in CRDs, `kubebuilder` auto-generates the `DeepCopyInto` method,
// which isn't possible for types from external packages.
// IMPORTANT: Run "make generate" and "make manifests" to regenerate code after modifying this file

type (
	ConfigToUpdate struct {
		Backend     *map[string]Empty        `json:"backend,omitempty"`
		Mirror      *MirrorConfToUpdate      `json:"mirror,omitempty"`
		EC          *ECConfToUpdate          `json:"ec,omitempty"`
		GetBatch    *GetBatchConfToUpdate    `json:"get_batch,omitempty"`
		Log         *LogConfToUpdate         `json:"log,omitempty"`
		Periodic    *PeriodConfToUpdate      `json:"periodic,omitempty"`
		Tracing     *TracingConfToUpdate     `json:"tracing,omitempty"`
		Timeout     *TimeoutConfToUpdate     `json:"timeout,omitempty"`
		Client      *ClientConfToUpdate      `json:"client,omitempty"`
		Space       *SpaceConfToUpdate       `json:"space,omitempty"`
		LRU         *LRUConfToUpdate         `json:"lru,omitempty"`
		Disk        *DiskConfToUpdate        `json:"disk,omitempty"`
		Rebalance   *RebalanceConfToUpdate   `json:"rebalance,omitempty"`
		Resilver    *ResilverConfToUpdate    `json:"resilver,omitempty"`
		Cksum       *CksumConfToUpdate       `json:"checksum,omitempty"`
		Versioning  *VersionConfToUpdate     `json:"versioning,omitempty"`
		Net         *NetConfToUpdate         `json:"net,omitempty"`
		FSHC        *FSHCConfToUpdate        `json:"fshc,omitempty"`
		Chunks      *ChunksConfToUpdate      `json:"chunks,omitempty"`
		Auth        *AuthConfToUpdate        `json:"auth,omitempty"`
		Keepalive   *KeepaliveConfToUpdate   `json:"keepalivetracker,omitempty"`
		Downloader  *DownloaderConfToUpdate  `json:"downloader,omitempty"`
		DSort       *DSortConfToUpdate       `json:"distributed_sort,omitempty"`
		Transport   *TransportConfToUpdate   `json:"transport,omitempty"`
		Memsys      *MemsysConfToUpdate      `json:"memsys,omitempty"`
		TCB         *TCBConfToUpdate         `json:"tcb,omitempty"`
		TCO         *TCOConfToUpdate         `json:"tco,omitempty"`
		Arch        *ArchConfToUpdate        `json:"arch,omitempty"`
		Lso         *LsoConfToUpdate         `json:"lso,omitempty"`
		WritePolicy *WritePolicyConfToUpdate `json:"write_policy,omitempty"`
		Proxy       *ProxyConfToUpdate       `json:"proxy,omitempty"`
		RateLimit   *RateLimitConfToUpdate   `json:"rate_limit,omitempty"`
		Features    *string                  `json:"features,omitempty"`
	}
	XactConfToUpdate struct {
		Compression *string `json:"compression,omitempty"`
		SbundleMult *int    `json:"bundle_multiplier,omitempty"`
		Burst       *int    `json:"burst_buffer,omitempty"`
	}
	MirrorConfToUpdate struct {
		Enabled *bool  `json:"enabled,omitempty"`
		Copies  *int64 `json:"copies,omitempty"`
		Burst   *int   `json:"burst_buffer,omitempty"`
	}
	ECConfToUpdate struct {
		XactConfToUpdate `json:",inline"`
		ObjSizeLimit     *int64 `json:"objsize_limit,omitempty"`
		DataSlices       *int   `json:"data_slices,omitempty"`
		ParitySlices     *int   `json:"parity_slices,omitempty"`
		Enabled          *bool  `json:"enabled,omitempty"`
		DiskOnly         *bool  `json:"disk_only,omitempty"`
	}
	GetBatchConfToUpdate struct {
		XactConfToUpdate `json:",inline"`
		MaxWait          *Duration `json:"max_wait,omitempty"`
		NumWarmupWorkers *int      `json:"warmup_workers,omitempty"`
		MaxSoftErrs      *int      `json:"max_soft_errs,omitempty"`
		MaxGFN           *int      `json:"max_gfn,omitempty"`
	}
	LogConfToUpdate struct {
		Level     *aiscos.LogLevel `json:"level,omitempty"`
		ToStderr  *bool            `json:"to_stderr,omitempty"`
		MaxSize   *SizeIEC         `json:"max_size,omitempty"`
		MaxTotal  *SizeIEC         `json:"max_total,omitempty"`
		FlushTime *Duration        `json:"flush_time,omitempty"`
		StatsTime *Duration        `json:"stats_time,omitempty"`
	}
	PeriodConfToUpdate struct {
		StatsTime     *Duration `json:"stats_time,omitempty"`
		RetrySyncTime *Duration `json:"retry_sync_time,omitempty"`
		NotifTime     *Duration `json:"notif_time,omitempty"`
	}

	// NOTE: Updating TracingConfig requires daemon restart.
	TracingConfToUpdate struct {
		ExporterEndpoint      *string                        `json:"exporter_endpoint,omitempty"`   // gRPC exporter endpoint
		ExporterAuth          *TraceExporterAuthConfToUpdate `json:"exporter_auth,omitempty"`       // exporter auth config
		ServiceNamePrefix     *string                        `json:"service_name_prefix,omitempty"` // service name used by trace exporter
		ExtraAttributes       map[string]string              `json:"attributes,omitempty"`          // any extra-attributes to be added to traces
		SamplerProbabilityStr *string                        `json:"sampler_probability,omitempty"` // percentage of traces to be sampled
		Enabled               *bool                          `json:"enabled,omitempty"`
		SkipVerify            *bool                          `json:"skip_verify,omitempty"` // allow insecure exporter gRPC connection
	}
	TraceExporterAuthConfToUpdate struct {
		TokenHeader *string `json:"token_header,omitempty"` // header used to pass exporter auth token
		TokenFile   *string `json:"token_file,omitempty"`   // filepath from where auth token can be obtained
	}
	TimeoutConfToUpdate struct {
		CplaneOperation *Duration `json:"cplane_operation,omitempty" list:"readonly"`
		MaxKeepalive    *Duration `json:"max_keepalive,omitempty" list:"readonly"`
		MaxHostBusy     *Duration `json:"max_host_busy,omitempty"`
		Startup         *Duration `json:"startup_time,omitempty"`
		JoinAtStartup   *Duration `json:"join_startup_time,omitempty"`
		SendFile        *Duration `json:"send_file_time,omitempty"`
		EcStreams       *Duration `json:"ec_streams_time,omitempty"`
		ObjectMD        *Duration `json:"object_md,omitempty"`
		ColdGetConflict *Duration `json:"cold_get_conflict,omitempty"`
	}
	ClientConfToUpdate struct {
		Timeout        *Duration `json:"client_timeout,omitempty"`
		TimeoutLong    *Duration `json:"client_long_timeout,omitempty"`
		ListObjTimeout *Duration `json:"list_timeout,omitempty"`
	}
	ProxyConfToUpdate struct {
		PrimaryURL   *string `json:"primary_url,omitempty"`
		OriginalURL  *string `json:"original_url,omitempty"`
		DiscoveryURL *string `json:"discovery_url,omitempty"`
	}
	SpaceConfToUpdate struct {
		//+kubebuilder:validation:Minimum=0
		//+kubebuilder:validation:Maximum=100
		CleanupWM *int64 `json:"cleanupwm,omitempty"`
		//+kubebuilder:validation:Minimum=0
		//+kubebuilder:validation:Maximum=100
		LowWM *int64 `json:"lowwm,omitempty"`
		//+kubebuilder:validation:Minimum=0
		//+kubebuilder:validation:Maximum=100
		HighWM *int64 `json:"highwm,omitempty"`
		//+kubebuilder:validation:Minimum=0
		//+kubebuilder:validation:Maximum=100
		OOS             *int64    `json:"out_of_space,omitempty"`
		BatchSize       *int64    `json:"batch_size,omitempty"`
		DontCleanupTime *Duration `json:"dont_cleanup_time,omitempty"`
	}
	LRUConfToUpdate struct {
		Enabled         *bool     `json:"enabled,omitempty"`
		DontEvictTime   *Duration `json:"dont_evict_time,omitempty"`
		CapacityUpdTime *Duration `json:"capacity_upd_time,omitempty"`
		BatchSize       *int64    `json:"batch_size,omitempty"`
	}
	DiskConfToUpdate struct {
		DiskUtilLowWM    *int64    `json:"disk_util_low_wm,omitempty"`
		DiskUtilHighWM   *int64    `json:"disk_util_high_wm,omitempty"`
		DiskUtilMaxWM    *int64    `json:"disk_util_max_wm,omitempty"`
		IostatTimeLong   *Duration `json:"iostat_time_long,omitempty"`
		IostatTimeShort  *Duration `json:"iostat_time_short,omitempty"`
		IostatTimeSmooth *Duration `json:"iostat_time_smooth,omitempty"`
	}
	RebalanceConfToUpdate struct {
		XactConfToUpdate `json:",inline"`
		Enabled          *bool     `json:"enabled,omitempty"`
		DestRetryTime    *Duration `json:"dest_retry_time,omitempty"`
	}
	ResilverConfToUpdate struct {
		Enabled *bool `json:"enabled,omitempty"` // true=auto-resilver | manual resilvering
	}
	CksumConfToUpdate struct {
		Type            *string `json:"type,omitempty"`
		ValidateColdGet *bool   `json:"validate_cold_get,omitempty"`
		ValidateWarmGet *bool   `json:"validate_warm_get,omitempty"`
		ValidateObjMove *bool   `json:"validate_obj_move,omitempty"`
		EnableReadRange *bool   `json:"enable_read_range,omitempty"`
	}
	VersionConfToUpdate struct {
		Enabled         *bool `json:"enabled,omitempty"`
		ValidateWarmGet *bool `json:"validate_warm_get,omitempty"`
		Sync            *bool `json:"synchronize,omitempty"`
	}
	NetConfToUpdate struct {
		HTTP    *HTTPConfToUpdate `json:"http,omitempty"`
		UseIPv6 *bool             `json:"use_ipv6,omitempty"`
	}

	TLSConfToUpdate struct {
		Certificate   *string `json:"server_crt,omitempty"`
		CertKey       *string `json:"server_key,omitempty"`
		ClientCA      *string `json:"client_ca_tls,omitempty"`
		ClientAuthTLS *int    `json:"client_auth_tls,omitempty"`
	}

	HTTPConfToUpdate struct {
		Certificate            *string   `json:"server_crt,omitempty"`
		CertKey                *string   `json:"server_key,omitempty"`
		ServerNameTLS          *string   `json:"domain_tls,omitempty"`
		ClientCA               *string   `json:"client_ca_tls,omitempty"`
		IdleConnTimeout        *Duration `json:"idle_conn_time,omitempty"`
		BackendIdleConnTimeout *Duration `json:"backend_idle_conn_time,omitempty"`
		MaxIdleConnsPerHost    *int      `json:"idle_conns_per_host,omitempty"`
		MaxIdleConns           *int      `json:"idle_conns,omitempty"`
		WriteBufferSize        *int      `json:"write_buffer_size,omitempty" list:"readonly"`
		ReadBufferSize         *int      `json:"read_buffer_size,omitempty" list:"readonly"`
		ClientAuthTLS          *int      `json:"client_auth_tls,omitempty"`
		UseHTTPS               *bool     `json:"use_https,omitempty"`
		SkipVerifyCrt          *bool     `json:"skip_verify,omitempty"`
		Chunked                *bool     `json:"chunked_transfer,omitempty"`
		// Optional TLS overrides for public listeners.
		Pub *TLSConfToUpdate `json:"pub,omitempty"`
	}
	FSHCConfToUpdate struct {
		TestFileCount *int      `json:"test_files,omitempty"`
		HardErrs      *int      `json:"error_limit,omitempty"`
		IOErrs        *int      `json:"io_err_limit,omitempty"`
		IOErrTime     *Duration `json:"io_err_time,omitempty"`
		Enabled       *bool     `json:"enabled,omitempty"`
	}
	ChunksConfToUpdate struct {
		ObjSizeLimit      *SizeIEC `json:"objsize_limit,omitempty"`
		MaxMonolithicSize *SizeIEC `json:"max_monolithic_size,omitempty"`
		ChunkSize         *SizeIEC `json:"chunk_size,omitempty"`
		CheckpointEvery   *int     `json:"checkpoint_every,omitempty"`
		Flags             *uint64  `json:"flags,omitempty"`
	}
	AuthConfToUpdate struct {
		// Deprecated: use ClientAuthRequired.
		Enabled            *bool                       `json:"enabled,omitempty"`
		ClientAuthRequired *bool                       `json:"client_auth_required,omitempty"`
		Signature          *AuthSignatureConfToUpdate  `json:"signature,omitempty"`
		RequiredClaims     *RequiredClaimsConfToUpdate `json:"required_claims,omitempty"`
		OIDC               *OIDCConfToUpdate           `json:"oidc,omitempty"`
		// Deprecated: use IntraCluster.
		ClusterKey   *ClusterKeyConfToUpdate   `json:"cluster_key,omitempty"`
		IntraCluster *IntraClusterConfToUpdate `json:"intra_cluster,omitempty"`
	}
	AuthSignatureConfToUpdate struct {
		Key    *string `json:"key,omitempty"`
		Method *string `json:"method,omitempty"`
	}
	RequiredClaimsConfToUpdate struct {
		Aud *[]string `json:"aud,omitempty"`
	}
	OIDCConfToUpdate struct {
		AllowedIssuers *[]string              `json:"allowed_iss,omitempty"`
		IssuerCA       *string                `json:"issuer_ca_bundle,omitempty"`
		JWKSCacheConf  *JWKSCacheConfToUpdate `json:"jwks_cache,omitempty"`
	}
	JWKSCacheConfToUpdate struct {
		MinRotationRefresh   *Duration `json:"min_rotation_refresh,omitempty"`
		MinBackgroundRefresh *Duration `json:"min_background_refresh,omitempty"`
	}
	// Deprecated: use IntraClusterConfToUpdate.
	ClusterKeyConfToUpdate struct {
		Enabled       *bool     `json:"enabled,omitempty"`
		TTL           *Duration `json:"ttl,omitempty"`
		NonceWindow   *Duration `json:"nonce_window,omitempty"`
		RotationGrace *Duration `json:"rotation_grace,omitempty"`
	}
	IntraClusterConfToUpdate struct {
		NodeJoinSecretPath *string   `json:"node_join_secret_path,omitempty"`
		TTL                *Duration `json:"ttl,omitempty"`
		NonceWindow        *Duration `json:"nonce_window,omitempty"`
		RotationGrace      *Duration `json:"rotation_grace,omitempty"`
		RequestAuth        *bool     `json:"request_auth,omitempty"`
	}
	KeepaliveTrackerConfToUpdate struct {
		Interval *Duration `json:"interval,omitempty"`
		Name     *string   `json:"name,omitempty"`
		Factor   *uint8    `json:"factor,omitempty"`
	}
	KeepaliveConfToUpdate struct {
		Proxy       *KeepaliveTrackerConfToUpdate `json:"proxy,omitempty"`
		Target      *KeepaliveTrackerConfToUpdate `json:"target,omitempty"`
		NumRetries  *int                          `json:"num_retries,omitempty"`
		RetryFactor *uint8                        `json:"retry_factor,omitempty"`
	}
	DownloaderConfToUpdate struct {
		Timeout *Duration `json:"timeout,omitempty"`
	}
	DSortConfToUpdate struct {
		XactConfToUpdate    `json:",inline"`
		DuplicatedRecords   *string   `json:"duplicated_records,omitempty"`
		MissingShards       *string   `json:"missing_shards,omitempty"`
		EKMMalformedLine    *string   `json:"ekm_malformed_line,omitempty"`
		EKMMissingKey       *string   `json:"ekm_missing_key,omitempty"`
		DefaultMaxMemUsage  *string   `json:"default_max_mem_usage,omitempty"`
		CallTimeout         *Duration `json:"call_timeout,omitempty"`
		DSorterMemThreshold *string   `json:"dsorter_mem_threshold,omitempty"`
	}
	TransportConfToUpdate struct {
		MaxHeaderSize    *int      `json:"max_header,omitempty" list:"readonly"`
		Burst            *int      `json:"burst_buffer,omitempty" list:"readonly"`
		IdleTeardown     *Duration `json:"idle_teardown,omitempty"`
		QuiesceTime      *Duration `json:"quiescent,omitempty"`
		LZ4BlockMaxSize  *SizeIEC  `json:"lz4_block,omitempty"`
		LZ4FrameChecksum *bool     `json:"lz4_frame_checksum,omitempty"`
	}
	MemsysConfToUpdate struct {
		MinFree        *SizeIEC  `json:"min_free,omitempty" list:"readonly"`
		DefaultBufSize *SizeIEC  `json:"default_buf,omitempty"`
		SizeToGC       *SizeIEC  `json:"to_gc,omitempty"`
		HousekeepTime  *Duration `json:"hk_time,omitempty"`
		MinPctTotal    *int      `json:"min_pct_total,omitempty" list:"readonly"`
		MinPctFree     *int      `json:"min_pct_free,omitempty" list:"readonly"`
	}
	TCBConfToUpdate struct {
		XactConfToUpdate `json:",inline"`
	}
	TCOConfToUpdate struct {
		XactConfToUpdate `json:",inline"`
	}
	ArchConfToUpdate struct {
		XactConfToUpdate `json:",inline"`
	}
	LsoConfToUpdate struct {
		XactConfToUpdate `json:",inline"`
		WalkBuffer       *int      `json:"walk_buffer,omitempty"`
		IdleTime         *Duration `json:"idle_time,omitempty"`
		QuiesceTime      *Duration `json:"quiescent,omitempty"`
	}
	WritePolicyConfToUpdate struct {
		Data *string `json:"data,omitempty"`
		MD   *string `json:"md,omitempty"`
	}
	RateLimitBaseToUpdate struct {
		Verbs     *string   `json:"per_op_max_tokens,omitempty"`
		Interval  *Duration `json:"interval,omitempty"`
		MaxTokens *int      `json:"max_tokens,omitempty"`
		Enabled   *bool     `json:"enabled,omitempty"`
	}
	AdaptiveToUpdate struct {
		NumRetries            *int `json:"num_retries,omitempty"`
		RateLimitBaseToUpdate `json:",inline"`
	}
	BurstyToUpdate struct {
		Size                  *int `json:"burst_size,omitempty"`
		RateLimitBaseToUpdate `json:",inline"`
	}
	RateLimitConfToUpdate struct {
		Backend  *AdaptiveToUpdate `json:"backend,omitempty"`
		Frontend *BurstyToUpdate   `json:"frontend,omitempty"`
	}
)

func (c *ConfigToUpdate) IsRebalanceEnabledSet() bool {
	if c.Rebalance == nil {
		return false
	}
	return c.Rebalance.Enabled != nil
}

// RequiresClientAuth reports whether the AIS config requires authenticated client requests.
func (c *ConfigToUpdate) RequiresClientAuth() bool {
	if c == nil || c.Auth == nil {
		return false
	}
	if c.Auth.ClientAuthRequired != nil {
		return *c.Auth.ClientAuthRequired
	}
	return c.Auth.Enabled != nil && *c.Auth.Enabled
}

// ClientAuthRequiredSet reports whether the spec sets auth.client_auth_required.
func (c *ConfigToUpdate) ClientAuthRequiredSet() bool {
	return c != nil && c.Auth != nil && c.Auth.ClientAuthRequired != nil
}

// IntraClusterSet reports whether the spec sets auth.intra_cluster.
func (c *ConfigToUpdate) IntraClusterSet() bool {
	return c != nil && c.Auth != nil && c.Auth.IntraCluster != nil
}

// RequiredAudiences returns auth.required_claims.aud if set, or nil if not configured.
func (c *ConfigToUpdate) RequiredAudiences() []string {
	if c == nil || c.Auth == nil || c.Auth.RequiredClaims == nil || c.Auth.RequiredClaims.Aud == nil {
		return nil
	}
	return *c.Auth.RequiredClaims.Aud
}

// DeprecatedAuthMessages returns a message for each deprecated auth option set.
func (c *ConfigToUpdate) DeprecatedAuthMessages() []string {
	if c == nil || c.Auth == nil {
		return nil
	}
	var msgs []string
	if c.Auth.Enabled != nil {
		msgs = append(msgs, "spec.configToUpdate.auth.enabled is deprecated, use spec.configToUpdate.auth.client_auth_required")
	}
	if c.Auth.ClusterKey != nil {
		msgs = append(msgs, "spec.configToUpdate.auth.cluster_key is deprecated, use spec.configToUpdate.auth.intra_cluster")
	}
	return msgs
}

// RebalanceEnabled reports the configured rebalance.enabled value, defaulting to true when unset.
func (c *ConfigToUpdate) RebalanceEnabled() bool {
	if c == nil || c.Rebalance == nil || c.Rebalance.Enabled == nil {
		return true
	}
	return *c.Rebalance.Enabled
}

func (c *ConfigToUpdate) UpdateRebalanceEnabled(enabled *bool) {
	if c.Rebalance == nil {
		c.Rebalance = &RebalanceConfToUpdate{}
	}
	c.Rebalance.Enabled = enabled
}

func (c *ConfigToUpdate) ConfigureBackend(spec *AIStoreSpec) {
	if c.Backend == nil {
		m := make(map[string]Empty, 8)
		c.Backend = &m
	}
	backend := *c.Backend
	// If we have secrets with missing config entries, add them
	if spec.AWSSecretName != nil {
		backend[aisapc.AWS] = Empty{}
	}
	if spec.GCPSecretName != nil {
		backend[aisapc.GCP] = Empty{}
	}
	if spec.OCISecretName != nil {
		backend[aisapc.OCI] = Empty{}
	}
	if spec.HasAzureConfig() {
		backend[aisapc.Azure] = Empty{}
	}
}

func (c *ConfigToUpdate) EnsureHMACSignature() {
	if c.Auth == nil {
		c.Auth = &AuthConfToUpdate{}
	}
	if c.Auth.Signature == nil {
		// Associated Signature.Key is parsed via environment variable
		c.Auth.Signature = &AuthSignatureConfToUpdate{}
	}
	// AIS rejects a signature key without a method
	if c.Auth.Signature.Method == nil {
		c.Auth.Signature.Method = aisapc.Ptr(SigningKeyMethodHMAC)
	}
}

func (c *ConfigToUpdate) HasOIDCIssuers() bool {
	if c == nil || c.Auth == nil || c.Auth.OIDC == nil || c.Auth.OIDC.AllowedIssuers == nil {
		return false
	}
	return len(*c.Auth.OIDC.AllowedIssuers) > 0
}

func (c *ConfigToUpdate) ConfigureOIDCIssuer(issuerCAPath string) {
	if c.Auth == nil {
		c.Auth = &AuthConfToUpdate{}
	}
	if c.Auth.OIDC == nil {
		c.Auth.OIDC = &OIDCConfToUpdate{}
	}
	c.Auth.OIDC.IssuerCA = &issuerCAPath
}

func (c *ConfigToUpdate) Convert() (toUpdate *aiscmn.ConfigToSet, err error) {
	toUpdate = &aiscmn.ConfigToSet{}
	err = aiscos.MorphMarshal(c, toUpdate)
	return toUpdate, err
}
