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
		Compression *CompressionConfToUpdate `json:"compression,omitempty"`
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
		Copies      *int64 `json:"copies,omitempty"`
		Burst       *int   `json:"burst_buffer,omitempty"`
		UtilThresh  *int64 `json:"util_thresh,omitempty"`
		OptimizePUT *bool  `json:"optimize_put,omitempty"`
		Enabled     *bool  `json:"enabled,omitempty"`
	}
	ECConfToUpdate struct {
		Enabled      *bool   `json:"enabled,omitempty"`
		ObjSizeLimit *int64  `json:"objsize_limit,omitempty"`
		DataSlices   *int    `json:"data_slices,omitempty"`
		ParitySlices *int    `json:"parity_slices,omitempty"`
		Compression  *string `json:"compression,omitempty"`
		DiskOnly     *bool   `json:"disk_only,omitempty"`
	}
	LogConfToUpdate struct {
		Dir      *string `json:"dir,omitempty"`       // log directory
		Level    *string `json:"level,omitempty"`     // log level aka verbosity
		MaxSize  *uint64 `json:"max_size,omitempty"`  // size that triggers log rotation
		MaxTotal *uint64 `json:"max_total,omitempty"` // max total size of all the logs in the log directory
	}
	PeriodConfToUpdate struct {
		StatsTime     *cos.Duration `json:"stats_time,omitempty"`
		RetrySyncTime *cos.Duration `json:"retry_sync_time,omitempty"`
		NotifTime     *cos.Duration `json:"notif_time,omitempty"`
	}
	TimeoutConfToUpdate struct {
		CplaneOperation *cos.Duration `json:"cplane_operation,omitempty"`
		MaxKeepalive    *cos.Duration `json:"max_keepalive,omitempty"`
		MaxHostBusy     *cos.Duration `json:"max_host_busy,omitempty"`
		Startup         *cos.Duration `json:"startup_time,omitempty"`
		SendFile        *cos.Duration `json:"send_file_time,omitempty"`
		// v3.8
		TransportIdleTeardown *cos.Duration `json:"transport_idle_term,omitempty"`
	}
	ClientConfToUpdate struct {
		Timeout     *cos.Duration `json:"client_timeout,omitempty"`
		TimeoutLong *cos.Duration `json:"client_long_timeout,omitempty"`
		ListObjects *cos.Duration `json:"list_timeout,omitempty"`
	}
	ProxyConfToUpdate struct {
		PrimaryURL   *string `json:"primary_url,omitempty"`
		OriginalURL  *string `json:"original_url,omitempty"`
		DiscoveryURL *string `json:"discovery_url,omitempty"`
		NonElectable *bool   `json:"non_electable,omitempty"`
	}
	LRUConfToUpdate struct {
		LowWM           *int64        `json:"lowwm,omitempty"`
		HighWM          *int64        `json:"highwm,omitempty"`
		OOS             *int64        `json:"out_of_space,omitempty"`
		DontEvictTime   *cos.Duration `json:"dont_evict_time,omitempty"`
		CapacityUpdTime *cos.Duration `json:"capacity_upd_time,omitempty"`
		Enabled         *bool         `json:"enabled,omitempty"`
	}
	DiskConfToUpdate struct {
		DiskUtilLowWM   *int64        `json:"disk_util_low_wm,omitempty"`
		DiskUtilHighWM  *int64        `json:"disk_util_high_wm,omitempty"`
		DiskUtilMaxWM   *int64        `json:"disk_util_max_wm,omitempty"`
		IostatTimeLong  *cos.Duration `json:"iostat_time_long,omitempty"`
		IostatTimeShort *cos.Duration `json:"iostat_time_short,omitempty"`
	}
	RebalanceConfToUpdate struct {
		DestRetryTime *cos.Duration `json:"dest_retry_time,omitempty"`
		Quiesce       *cos.Duration `json:"quiescent,omitempty"`
		Compression   *string       `json:"compression,omitempty"`
		Multiplier    *uint8        `json:"multiplier,omitempty"`
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
		Key             *string `json:"server_key,omitempty"`
		WriteBufferSize *int    `json:"write_buffer_size,omitempty"`
		ReadBufferSize  *int    `json:"read_buffer_size,omitempty"`
		UseHTTPS        *bool   `json:"use_https,omitempty"`
		SkipVerify      *bool   `json:"skip_verify,omitempty"`
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
		Interval *cos.Duration `json:"interval,omitempty"`
		Name     *string       `json:"name,omitempty"`
		Factor   *uint8        `json:"factor,omitempty"`
	}
	KeepaliveConfToUpdate struct {
		Proxy         *KeepaliveTrackerConfToUpdate `json:"proxy,omitempty"`
		Target        *KeepaliveTrackerConfToUpdate `json:"target,omitempty"`
		RetryFactor   *uint8                        `json:"retry_factor,omitempty"`
		TimeoutFactor *uint8                        `json:"timeout_factor,omitempty"`
	}
	DownloaderConfToUpdate struct {
		Timeout *cos.Duration `json:"timeout,omitempty"`
	}
	DSortConfToUpdate struct {
		DuplicatedRecords   *string       `json:"duplicated_records,omitempty"`
		MissingShards       *string       `json:"missing_shards,omitempty"`
		EKMMalformedLine    *string       `json:"ekm_malformed_line,omitempty"`
		EKMMissingKey       *string       `json:"ekm_missing_key,omitempty"`
		DefaultMaxMemUsage  *string       `json:"default_max_mem_usage,omitempty"`
		CallTimeout         *cos.Duration `json:"call_timeout,omitempty"`
		Compression         *string       `json:"compression,omitempty"`
		DSorterMemThreshold *string       `json:"dsorter_mem_threshold,omitempty"`
	}
	CompressionConfToUpdate struct {
		BlockMaxSize *int  `json:"block_size,omitempty"`
		Checksum     *bool `json:"checksum,omitempty"`
	}
	WritePolicyConfToUpdate struct {
		Data *string `json:"data,omitempty"`
		MD   *string `json:"md,omitempty"`
	}
)
