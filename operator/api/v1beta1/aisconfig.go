// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2021-2022, NVIDIA CORPORATION. All rights reserved.
 */
package v1beta1

import (
	"crypto/sha256"
	"encoding/hex"

	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	jsoniter "github.com/json-iterator/go"
)

// NOTE: `*ToUpdate` structures are duplicates of `*ToUpdate` structs from AIStore main repository.
// For custom types used in CRDs, `kubebuilder` auto-generates the `DeepCopyInto` method,
// which isn't possible for types from external packages.
// IMPORTANT: Run "make" to regenerate code after modifying this file

type (
	ConfigToUpdate struct {
		Backend     *map[string]Empty        `json:"backend,omitempty"`
		Mirror      *MirrorConfToUpdate      `json:"mirror,omitempty"`
		EC          *ECConfToUpdate          `json:"ec,omitempty"`
		Log         *LogConfToUpdate         `json:"log,omitempty"`
		Periodic    *PeriodConfToUpdate      `json:"periodic,omitempty"`
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
		Auth        *AuthConfToUpdate        `json:"auth,omitempty"`
		Keepalive   *KeepaliveConfToUpdate   `json:"keepalivetracker,omitempty"`
		Downloader  *DownloaderConfToUpdate  `json:"downloader,omitempty"`
		DSort       *DSortConfToUpdate       `json:"distributed_sort,omitempty"`
		Transport   *TransportConfToUpdate   `json:"transport,omitempty"`
		Memsys      *MemsysConfToUpdate      `json:"memsys,omitempty"`
		TCB         *TCBConfToUpdate         `json:"tcb,omitempty"`
		WritePolicy *WritePolicyConfToUpdate `json:"write_policy,omitempty"`
		Proxy       *ProxyConfToUpdate       `json:"proxy,omitempty"`
		Features    *string                  `json:"features,omitempty"`
	}
	MirrorConfToUpdate struct {
		Enabled *bool  `json:"enabled,omitempty"`
		Copies  *int64 `json:"copies,omitempty"`
		Burst   *int   `json:"burst_buffer,omitempty"`
	}
	ECConfToUpdate struct {
		ObjSizeLimit *int64  `json:"objsize_limit,omitempty"`
		Compression  *string `json:"compression,omitempty"`
		SbundleMult  *int    `json:"bundle_multiplier,omitempty"`
		DataSlices   *int    `json:"data_slices,omitempty"`
		ParitySlices *int    `json:"parity_slices,omitempty"`
		Enabled      *bool   `json:"enabled,omitempty"`
		DiskOnly     *bool   `json:"disk_only,omitempty"`
	}
	LogConfToUpdate struct {
		Level     *string      `json:"level,omitempty"`
		ToStderr  *bool        `json:"to_stderr,omitempty"`
		MaxSize   *cos.SizeIEC `json:"max_size,omitempty"`
		MaxTotal  *cos.SizeIEC `json:"max_total,omitempty"`
		FlushTime *Duration    `json:"flush_time,omitempty"`
		StatsTime *Duration    `json:"stats_time,omitempty"`
	}
	PeriodConfToUpdate struct {
		StatsTime     *Duration `json:"stats_time,omitempty"`
		RetrySyncTime *Duration `json:"retry_sync_time,omitempty"`
		NotifTime     *Duration `json:"notif_time,omitempty"`
	}
	TimeoutConfToUpdate struct {
		CplaneOperation *Duration `json:"cplane_operation,omitempty" list:"readonly"`
		MaxKeepalive    *Duration `json:"max_keepalive,omitempty" list:"readonly"`
		MaxHostBusy     *Duration `json:"max_host_busy,omitempty"`
		Startup         *Duration `json:"startup_time,omitempty"`
		JoinAtStartup   *Duration `json:"join_startup_time,omitempty"`
		SendFile        *Duration `json:"send_file_time,omitempty"`
	}
	ClientConfToUpdate struct {
		Timeout     *Duration `json:"client_timeout,omitempty"`
		TimeoutLong *Duration `json:"client_long_timeout,omitempty"`
		ListObjects *Duration `json:"list_timeout,omitempty"`
	}
	ProxyConfToUpdate struct {
		PrimaryURL   *string `json:"primary_url,omitempty"`
		OriginalURL  *string `json:"original_url,omitempty"`
		DiscoveryURL *string `json:"discovery_url,omitempty"`
		NonElectable *bool   `json:"non_electable,omitempty"`
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
		OOS *int64 `json:"out_of_space,omitempty"`
	}
	LRUConfToUpdate struct {
		Enabled         *bool     `json:"enabled,omitempty"`
		DontEvictTime   *Duration `json:"dont_evict_time,omitempty"`
		CapacityUpdTime *Duration `json:"capacity_upd_time,omitempty"`
	}
	DiskConfToUpdate struct {
		DiskUtilLowWM   *int64    `json:"disk_util_low_wm,omitempty"`
		DiskUtilHighWM  *int64    `json:"disk_util_high_wm,omitempty"`
		DiskUtilMaxWM   *int64    `json:"disk_util_max_wm,omitempty"`
		IostatTimeLong  *Duration `json:"iostat_time_long,omitempty"`
		IostatTimeShort *Duration `json:"iostat_time_short,omitempty"`
	}
	RebalanceConfToUpdate struct {
		Enabled       *bool     `json:"enabled,omitempty"`
		DestRetryTime *Duration `json:"dest_retry_time,omitempty"`
		Compression   *string   `json:"compression,omitempty"`
		SbundleMult   *int      `json:"bundle_multiplier,omitempty"`
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
	}
	NetConfToUpdate struct {
		HTTP *HTTPConfToUpdate `json:"http,omitempty"`
	}
	HTTPConfToUpdate struct {
		Certificate     *string `json:"server_crt,omitempty"`
		CertKey         *string `json:"server_key,omitempty"`
		ServerNameTLS   *string `json:"domain_tls,omitempty"`
		ClientCA        *string `json:"client_ca_tls,omitempty"`
		WriteBufferSize *int    `json:"write_buffer_size,omitempty" list:"readonly"`
		ReadBufferSize  *int    `json:"read_buffer_size,omitempty" list:"readonly"`
		ClientAuthTLS   *int    `json:"client_auth_tls,omitempty"`
		UseHTTPS        *bool   `json:"use_https,omitempty"`
		SkipVerifyCrt   *bool   `json:"skip_verify,omitempty"`
		Chunked         *bool   `json:"chunked_transfer,omitempty"`
	}
	FSHCConfToUpdate struct {
		TestFileCount *int          `json:"test_files,omitempty"`
		HardErrs      *int          `json:"error_limit,omitempty"`
		IOErrs        *int          `json:"io_err_limit,omitempty"`
		IOErrTime     *cos.Duration `json:"io_err_time,omitempty"`
		Enabled       *bool         `json:"enabled,omitempty"`
	}
	AuthConfToUpdate struct {
		Enabled *bool   `json:"enabled,omitempty"`
		Secret  *string `json:"secret,omitempty"`
	}
	KeepaliveTrackerConfToUpdate struct {
		Interval *Duration `json:"interval,omitempty"`
		Name     *string   `json:"name,omitempty"`
		Factor   *uint8    `json:"factor,omitempty"`
	}
	KeepaliveConfToUpdate struct {
		Proxy       *KeepaliveTrackerConfToUpdate `json:"proxy,omitempty"`
		Target      *KeepaliveTrackerConfToUpdate `json:"target,omitempty"`
		RetryFactor *uint8                        `json:"retry_factor,omitempty"`
	}
	DownloaderConfToUpdate struct {
		Timeout *Duration `json:"timeout,omitempty"`
	}
	DSortConfToUpdate struct {
		DuplicatedRecords   *string   `json:"duplicated_records,omitempty"`
		MissingShards       *string   `json:"missing_shards,omitempty"`
		EKMMalformedLine    *string   `json:"ekm_malformed_line,omitempty"`
		EKMMissingKey       *string   `json:"ekm_missing_key,omitempty"`
		DefaultMaxMemUsage  *string   `json:"default_max_mem_usage,omitempty"`
		CallTimeout         *Duration `json:"call_timeout,omitempty"`
		DSorterMemThreshold *string   `json:"dsorter_mem_threshold,omitempty"`
		Compression         *string   `json:"compression,omitempty"`
		SbundleMult         *int      `json:"bundle_multiplier,omitempty"`
	}
	TransportConfToUpdate struct {
		MaxHeaderSize    *int      `json:"max_header,omitempty" list:"readonly"`
		Burst            *int      `json:"burst_buffer,omitempty" list:"readonly"`
		IdleTeardown     *Duration `json:"idle_teardown,omitempty"`
		QuiesceTime      *Duration `json:"quiescent,omitempty"`
		LZ4BlockMaxSize  *int      `json:"lz4_block,omitempty"`
		LZ4FrameChecksum *bool     `json:"lz4_frame_checksum,omitempty"`
	}
	MemsysConfToUpdate struct {
		MinFree        *cos.SizeIEC `json:"min_free,omitempty" list:"readonly"`
		DefaultBufSize *cos.SizeIEC `json:"default_buf,omitempty"`
		SizeToGC       *cos.SizeIEC `json:"to_gc,omitempty"`
		HousekeepTime  *Duration    `json:"hk_time,omitempty"`
		MinPctTotal    *int         `json:"min_pct_total,omitempty" list:"readonly"`
		MinPctFree     *int         `json:"min_pct_free,omitempty" list:"readonly"`
	}
	TCBConfToUpdate struct {
		Compression *string `json:"compression,omitempty"`
		SbundleMult *int    `json:"bundle_multiplier,omitempty"`
	}
	WritePolicyConfToUpdate struct {
		Data *string `json:"data,omitempty"`
		MD   *string `json:"md,omitempty"`
	}
)

// UpdateRebalanceEnabled Sets the rebalance config to include the default value for `Rebalance.Enabled`
func (c *ConfigToUpdate) UpdateRebalanceEnabled(rebEnabled *bool) {
	if c.Rebalance == nil {
		c.Rebalance = &RebalanceConfToUpdate{}
	}
	// Allows for other rebalance config settings to be set, but ensures enabled is always included
	if c.Rebalance.Enabled == nil {
		c.Rebalance.Enabled = rebEnabled
	}
}

func (c *ConfigToUpdate) Hash() string {
	data, _ := jsoniter.Marshal(c)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (c *ConfigToUpdate) Convert() (toUpdate *aiscmn.ConfigToSet, err error) {
	toUpdate = &aiscmn.ConfigToSet{}
	err = cos.MorphMarshal(c, toUpdate)
	return toUpdate, err
}
