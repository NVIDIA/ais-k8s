// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package v1beta1

// NOTE: `*ToUpdate` structures are duplicates of `*ToUpdate` structs from AIStore main respoitory.
// For custom types used in CRDs, `kubebuilder` auto-generates the `DeepCopyInto` method, which isn't possible for types from external packages.
// IMPROTANT: Run "make" to regenerate code after modifying this file

type (
	ConfigToUpdate struct {
		Confdir     *string                  `json:"confdir,omitempty"`
		Backend     *map[string]Empty        `json:"backend,omitempty"`
		Mirror      *MirrorConfToUpdate      `json:"mirror,omitempty"`
		EC          *ECConfToUpdate          `json:"ec,omitempty"`
		Log         *LogConfToUpdate         `json:"log,omitempty"`
		Periodic    *PeriodConfToUpdate      `json:"periodic,omitempty"`
		Timeout     *TimeoutConfToUpdate     `json:"timeout,omitempty"`
		Client      *ClientConfToUpdate      `json:"client,omitempty"`
		LRU         *LRUConfToUpdate         `json:"lru,omitempty"`
		Disk        *DiskConfToUpdate        `json:"disk,omitempty"`
		Rebalance   *RebalanceConfToUpdate   `json:"rebalance,omitempty"`
		Replication *ReplicationConfToUpdate `json:"replication,omitempty"`
		Cksum       *CksumConfToUpdate       `json:"checksum,omitempty"`
		Versioning  *VersionConfToUpdate     `json:"versioning,omitempty"`
		Net         *NetConfToUpdate         `json:"net,omitempty"`
		FSHC        *FSHCConfToUpdate        `json:"fshc,omitempty"`
		Auth        *AuthConfToUpdate        `json:"auth,omitempty"`
		Keepalive   *KeepaliveConfToUpdate   `json:"keepalivetracker,omitempty"`
		Downloader  *DownloaderConfToUpdate  `json:"downloader,omitempty"`
		DSort       *DSortConfToUpdate       `json:"distributed_sort,omitempty"`
		Compression *CompressionConfToUpdate `json:"compression,omitempty"`
		MDWrite     *string                  `json:"md_write,omitempty"`

		// Logging
		LogLevel *string `json:"log_level,omitempty" copy:"skip"`
		Vmodule  *string `json:"vmodule,omitempty" copy:"skip"`
	}

	// TODO -- FIXME: Declaring map[string]struct{} / map[string]interface{}
	// raises error "name requested for invalid type: struct{}/interface{}"
	Empty              struct{}
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
	}
	LogConfToUpdate struct {
		Dir      *string `json:"dir,omitempty"`       // log directory
		Level    *string `json:"level,omitempty"`     // log level aka verbosity
		MaxSize  *uint64 `json:"max_size,omitempty"`  // size that triggers log rotation
		MaxTotal *uint64 `json:"max_total,omitempty"` // max total size of all the logs in the log directory
	}
	PeriodConfToUpdate struct {
		StatsTimeStr     *string `json:"stats_time,omitempty"`
		RetrySyncTimeStr *string `json:"retry_sync_time,omitempty"`
		NotifTimeStr     *string `json:"notif_time,omitempty"`
	}
	TimeoutConfToUpdate struct {
		CplaneOperationStr *string `json:"cplane_operation,omitempty"`
		MaxKeepaliveStr    *string `json:"max_keepalive,omitempty"`
		MaxHostBusyStr     *string `json:"max_host_busy,omitempty"`
		StartupStr         *string `json:"startup_time,omitempty"`
		SendFileStr        *string `json:"send_file_time,omitempty"`
	}
	ClientConfToUpdate struct {
		TimeoutStr     *string `json:"client_timeout,omitempty"`
		TimeoutLongStr *string `json:"client_long_timeout,omitempty"`
		ListObjectsStr *string `json:"list_timeout,omitempty"`
		Features       *string `json:"features,omitempty"`
	}
	LRUConfToUpdate struct {
		LowWM              *int64  `json:"lowwm,omitempty"`
		HighWM             *int64  `json:"highwm,omitempty"`
		OOS                *int64  `json:"out_of_space,omitempty"`
		DontEvictTimeStr   *string `json:"dont_evict_time,omitempty"`
		CapacityUpdTimeStr *string `json:"capacity_upd_time,omitempty"`
		Enabled            *bool   `json:"enabled,omitempty"`
	}
	DiskConfToUpdate struct {
		DiskUtilLowWM      *int64  `json:"disk_util_low_wm,omitempty"`  // no throttling below
		DiskUtilHighWM     *int64  `json:"disk_util_high_wm,omitempty"` // throttle longer when above
		DiskUtilMaxWM      *int64  `json:"disk_util_max_wm,omitempty"`
		IostatTimeLongStr  *string `json:"iostat_time_long,omitempty"`
		IostatTimeShortStr *string `json:"iostat_time_short,omitempty"`
	}
	RebalanceConfToUpdate struct {
		DestRetryTimeStr *string `json:"dest_retry_time,omitempty"` // max wait for ACKs & neighbors to complete
		QuiesceStr       *string `json:"quiescent,omitempty"`       // max wait for no-obj before next stage/batch
		Compression      *string `json:"compression,omitempty"`     // see CompressAlways, etc. enum
		Multiplier       *uint8  `json:"multiplier,omitempty"`      // stream-bundle-and-jogger multiplier
		Enabled          *bool   `json:"enabled,omitempty"`         // true=auto-rebalance | manual rebalancing
	}
	ReplicationConfToUpdate struct {
		OnColdGet     *bool `json:"on_cold_get,omitempty"`     // object replication on cold GET request
		OnPut         *bool `json:"on_put,omitempty"`          // object replication on PUT request
		OnLRUEviction *bool `json:"on_lru_eviction,omitempty"` // object replication on LRU eviction
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
		L4   *L4ConfToUpdate   `json:"l4,omitempty"`
		HTTP *HTTPConfToUpdate `json:"http,omitempty"`
	}
	L4ConfToUpdate struct {
		Proto               *string `json:"proto,omitempty"`              // tcp, udp
		PortStr             *string `json:"port,omitempty"`               // listening port
		PortIntraControlStr *string `json:"port_intra_control,omitempty"` // listening port for intra control network
		PortIntraDataStr    *string `json:"port_intra_data,omitempty"`    // listening port for intra data network
		SndRcvBufSize       *int    `json:"sndrcv_buf_size,omitempty"`    // SO_RCVBUF and SO_SNDBUF
	}
	HTTPConfToUpdate struct {
		Certificate     *string `json:"server_crt,omitempty"` // HTTPS: openssl certificate
		Key             *string `json:"server_key,omitempty"` // HTTPS: openssl key
		WriteBufferSize *int    `json:"write_buffer_size,omitempty"`
		ReadBufferSize  *int    `json:"read_buffer_size,omitempty"`
		UseHTTPS        *bool   `json:"use_https,omitempty"` // use HTTPS instead of HTTP
		SkipVerify      *bool   `json:"skip_verify,omitempty"`
		Chunked         *bool   `json:"chunked_transfer,omitempty"` // https://tools.ietf.org/html/rfc7230#page-36
	}
	FSHCConfToUpdate struct {
		TestFileCount *int  `json:"test_files,omitempty"`  // number of files to read/write
		ErrorLimit    *int  `json:"error_limit,omitempty"` // exceeding err limit causes disabling mountpath
		Enabled       *bool `json:"enabled,omitempty"`
	}
	AuthConfToUpdate struct {
		Secret  *string `json:"secret,omitempty"`
		Enabled *bool   `json:"enabled,omitempty"`
	}
	KeepaliveTrackerConfToUpdate struct {
		IntervalStr *string `json:"interval,omitempty"` // keepalive interval
		Name        *string `json:"name,omitempty"`     // "heartbeat", "average"
		Factor      *uint8  `json:"factor,omitempty"`   // only average
	}
	KeepaliveConfToUpdate struct {
		Proxy         *KeepaliveTrackerConfToUpdate `json:"proxy,omitempty"`  // how proxy tracks target keepalives
		Target        *KeepaliveTrackerConfToUpdate `json:"target,omitempty"` // how target tracks primary proxies keepalives
		RetryFactor   *uint8                        `json:"retry_factor,omitempty"`
		TimeoutFactor *uint8                        `json:"timeout_factor,omitempty"`
	}
	DownloaderConfToUpdate struct {
		TimeoutStr *string `json:"timeout,omitempty"`
	}
	DSortConfToUpdate struct {
		DuplicatedRecords   *string `json:"duplicated_records,omitempty"`
		MissingShards       *string `json:"missing_shards,omitempty"`
		EKMMalformedLine    *string `json:"ekm_malformed_line,omitempty"`
		EKMMissingKey       *string `json:"ekm_missing_key,omitempty"`
		DefaultMaxMemUsage  *string `json:"default_max_mem_usage,omitempty"`
		CallTimeoutStr      *string `json:"call_timeout,omitempty"`
		Compression         *string `json:"compression,omitempty"`
		DSorterMemThreshold *string `json:"dsorter_mem_threshold,omitempty"`
	}
	CompressionConfToUpdate struct {
		BlockMaxSize *int  `json:"block_size,omitempty"`
		Checksum     *bool `json:"checksum,omitempty"`
	}
)
