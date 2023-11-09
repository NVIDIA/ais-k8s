// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2021-2022, NVIDIA CORPORATION. All rights reserved.
 */
package v1beta1

import (
	"github.com/NVIDIA/aistore/cmn/cos"
	"github.com/NVIDIA/aistore/cmn/feat"
)

// NOTE: `*ToUpdate` structures are duplicates of `*ToUpdate` structs from AIStore main respoitory.
// For custom types used in CRDs, `kubebuilder` auto-generates the `DeepCopyInto` method, which isn't possible for types from external packages.
// IMPROTANT: Run "make" to regenerate code after modifying this file

type (
	ConfigToUpdate struct {
		// ClusterConfig
		Backend     *BackendConfToUpdate     `json:"backend,omitempty"`
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
		Features    *feat.Flags              `json:"features,string,omitempty"`
	}
	// TODO -- FIXME: Declaring map[string]struct{} / map[string]interface{}
	// raises error "name requested for invalid type: struct{}/interface{}"
	Empty               struct{}
	BackendConfToUpdate struct {
		Conf *map[string]Empty `json:"conf,omitempty"` // implementation depends on backend provider
	}
	MirrorConfToUpdate struct {
		Copies  *int64 `json:"copies,omitempty"`
		Burst   *int   `json:"burst_buffer,omitempty"`
		Enabled *bool  `json:"enabled,omitempty"`
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
		Level    *string      `json:"level,omitempty"`
		MaxSize  *cos.SizeIEC `json:"max_size,omitempty"`
		MaxTotal *cos.SizeIEC `json:"max_total,omitempty"`
		// Elapsed time (nanoseconds).
		FlushTime *cos.Duration `json:"flush_time,omitempty"`
		// Elapsed time (nanoseconds).
		StatsTime *cos.Duration `json:"stats_time,omitempty"`
	}
	PeriodConfToUpdate struct {
		// Elapsed time (nanoseconds).
		StatsTime *cos.Duration `json:"stats_time,omitempty"`
		// Elapsed time (nanoseconds).
		RetrySyncTime *cos.Duration `json:"retry_sync_time,omitempty"`
		// Elapsed time (nanoseconds).
		NotifTime *cos.Duration `json:"notif_time,omitempty"`
	}
	TimeoutConfToUpdate struct {
		// Elapsed time (nanoseconds).
		CplaneOperation *cos.Duration `json:"cplane_operation,omitempty" list:"readonly"`
		// Elapsed time (nanoseconds).
		MaxKeepalive *cos.Duration `json:"max_keepalive,omitempty" list:"readonly"`
		// Elapsed time (nanoseconds).
		MaxHostBusy *cos.Duration `json:"max_host_busy,omitempty"`
		// Elapsed time (nanoseconds).
		Startup *cos.Duration `json:"startup_time,omitempty"`
		// Elapsed time (nanoseconds).
		JoinAtStartup *cos.Duration `json:"join_startup_time,omitempty"`
		// Elapsed time (nanoseconds).
		SendFile *cos.Duration `json:"send_file_time,omitempty"`
	}
	ClientConfToUpdate struct {
		// Elapsed time (nanoseconds).
		Timeout *cos.Duration `json:"client_timeout,omitempty"`
		// Elapsed time (nanoseconds).
		TimeoutLong *cos.Duration `json:"client_long_timeout,omitempty"`
		// Elapsed time (nanoseconds).
		ListObjects *cos.Duration `json:"list_timeout,omitempty"`
	}
	ProxyConfToUpdate struct {
		PrimaryURL   *string `json:"primary_url,omitempty"`
		OriginalURL  *string `json:"original_url,omitempty"`
		DiscoveryURL *string `json:"discovery_url,omitempty"`
		NonElectable *bool   `json:"non_electable,omitempty"`
	}
	SpaceConfToUpdate struct {
		CleanupWM *int64 `json:"cleanupwm,omitempty"`
		LowWM     *int64 `json:"lowwm,omitempty"`
		HighWM    *int64 `json:"highwm,omitempty"`
		OOS       *int64 `json:"out_of_space,omitempty"`
	}
	LRUConfToUpdate struct {
		// Elapsed time (nanoseconds).
		DontEvictTime *cos.Duration `json:"dont_evict_time,omitempty"`
		// Elapsed time (nanoseconds).
		CapacityUpdTime *cos.Duration `json:"capacity_upd_time,omitempty"`
		Enabled         *bool         `json:"enabled,omitempty"`
	}
	DiskConfToUpdate struct {
		DiskUtilLowWM  *int64 `json:"disk_util_low_wm,omitempty"`
		DiskUtilHighWM *int64 `json:"disk_util_high_wm,omitempty"`
		DiskUtilMaxWM  *int64 `json:"disk_util_max_wm,omitempty"`
		// Elapsed time (nanoseconds).
		IostatTimeLong *cos.Duration `json:"iostat_time_long,omitempty"`
		// Elapsed time (nanoseconds).
		IostatTimeShort *cos.Duration `json:"iostat_time_short,omitempty"`
	}
	RebalanceConfToUpdate struct {
		// Elapsed time (nanoseconds).
		DestRetryTime *cos.Duration `json:"dest_retry_time,omitempty"`
		Compression   *string       `json:"compression,omitempty"`
		SbundleMult   *int          `json:"bundle_multiplier"`
		Enabled       *bool         `json:"enabled,omitempty"`
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
		TestFileCount *int  `json:"test_files,omitempty"`
		ErrorLimit    *int  `json:"error_limit,omitempty"`
		Enabled       *bool `json:"enabled,omitempty"`
	}
	AuthConfToUpdate struct {
		Secret  *string `json:"secret,omitempty"`
		Enabled *bool   `json:"enabled,omitempty"`
	}
	KeepaliveTrackerConfToUpdate struct {
		// Elapsed time (nanoseconds).
		Interval *cos.Duration `json:"interval,omitempty"`
		Name     *string       `json:"name,omitempty"`
		Factor   *uint8        `json:"factor,omitempty"`
	}
	KeepaliveConfToUpdate struct {
		Proxy       *KeepaliveTrackerConfToUpdate `json:"proxy,omitempty"`
		Target      *KeepaliveTrackerConfToUpdate `json:"target,omitempty"`
		RetryFactor *uint8                        `json:"retry_factor,omitempty"`
	}
	DownloaderConfToUpdate struct {
		// Elapsed time (nanoseconds).
		Timeout *cos.Duration `json:"timeout,omitempty"`
	}
	DSortConfToUpdate struct {
		DuplicatedRecords  *string `json:"duplicated_records,omitempty"`
		MissingShards      *string `json:"missing_shards,omitempty"`
		EKMMalformedLine   *string `json:"ekm_malformed_line,omitempty"`
		EKMMissingKey      *string `json:"ekm_missing_key,omitempty"`
		DefaultMaxMemUsage *string `json:"default_max_mem_usage,omitempty"`
		// Elapsed time (nanoseconds).
		CallTimeout         *cos.Duration `json:"call_timeout,omitempty"`
		DSorterMemThreshold *string       `json:"dsorter_mem_threshold,omitempty"`
		Compression         *string       `json:"compression,omitempty"`
		SbundleMult         *int          `json:"bundle_multiplier,omitempty"`
	}
	TransportConfToUpdate struct {
		MaxHeaderSize *int `json:"max_header,omitempty" list:"readonly"`
		Burst         *int `json:"burst_buffer,omitempty" list:"readonly"`
		// Elapsed time (nanoseconds).
		IdleTeardown *cos.Duration `json:"idle_teardown,omitempty"`
		// Elapsed time (nanoseconds).
		QuiesceTime      *cos.Duration `json:"quiescent,omitempty"`
		LZ4BlockMaxSize  *int          `json:"lz4_block,omitempty"`
		LZ4FrameChecksum *bool         `json:"lz4_frame_checksum,omitempty"`
	}
	MemsysConfToUpdate struct {
		MinFree        *cos.SizeIEC `json:"min_free,omitempty" list:"readonly"`
		DefaultBufSize *cos.SizeIEC `json:"default_buf,omitempty"`
		SizeToGC       *cos.SizeIEC `json:"to_gc,omitempty"`
		// Elapsed time (nanoseconds).
		HousekeepTime *cos.Duration `json:"hk_time,omitempty"`
		MinPctTotal   *int          `json:"min_pct_total,omitempty" list:"readonly"`
		MinPctFree    *int          `json:"min_pct_free,omitempty" list:"readonly"`
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
