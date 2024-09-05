// Package configs provides AIS cluster config types and defaults
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package configs

import (
	"time"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
)

var DefaultAuth = aiscmn.AuthConf{
	Enabled: false,
}

var DefaultCksum = aiscmn.CksumConf{
	Type:            cos.ChecksumXXHash,
	ValidateColdGet: false,
}

var DefaultClientConf = aiscmn.ClientConf{
	Timeout:        cos.Duration(10 * time.Second),
	TimeoutLong:    cos.Duration(5 * time.Minute),
	ListObjTimeout: cos.Duration(5 * time.Minute),
}

var DefaultTransport = aiscmn.TransportConf{
	MaxHeaderSize:   4096,
	Burst:           512,
	IdleTeardown:    cos.Duration(4 * time.Second),
	QuiesceTime:     cos.Duration(10 * time.Second),
	LZ4BlockMaxSize: cos.SizeIEC(256 * cos.KiB),
}

var DefaultTCB = aiscmn.TCBConf{
	Compression: aisapc.CompressNever,
	SbundleMult: 2,
}

var DefaultDisk = aiscmn.DiskConf{
	DiskUtilLowWM:   20,
	DiskUtilHighWM:  80,
	DiskUtilMaxWM:   95,
	IostatTimeLong:  cos.Duration(2 * time.Second),
	IostatTimeShort: cos.Duration(100 * time.Millisecond),
}

var DefaultNet = aiscmn.NetConf{
	L4: aiscmn.L4Conf{
		Proto: "tcp",
	},
	HTTP: aiscmn.HTTPConf{
		UseHTTPS: false,
		Chunked:  true,
	},
}

var DefaultFSHC = aiscmn.FSHCConf{
	TestFileCount: 4,
	HardErrs:      2,
	IOErrs:        10,
	IOErrTime:     cos.Duration(10 * time.Second),
	Enabled:       true,
}

var DefaultDsort = aiscmn.DsortConf{
	Compression:         aisapc.CompressNever,
	DuplicatedRecords:   aiscmn.IgnoreReaction,
	MissingShards:       aiscmn.IgnoreReaction,
	EKMMalformedLine:    aisapc.Abort,
	EKMMissingKey:       aisapc.Abort,
	DefaultMaxMemUsage:  "80%",
	DsorterMemThreshold: "100GB",
	CallTimeout:         cos.Duration(10 * time.Minute),
	SbundleMult:         2,
}

var DefaultDownloader = aiscmn.DownloaderConf{
	Timeout: cos.Duration(time.Hour),
}

var DefaultEC = aiscmn.ECConf{
	Enabled:      false,
	ObjSizeLimit: 262144,
	DataSlices:   2,
	ParitySlices: 2,
	SbundleMult:  2,
	Compression:  aisapc.CompressNever,
}

var DefaultKeepalive = aiscmn.KeepaliveConf{
	Proxy: aiscmn.KeepaliveTrackerConf{
		Interval: cos.Duration(10 * time.Second),
		Name:     "heartbeat",
		Factor:   3,
	},
	Target: aiscmn.KeepaliveTrackerConf{
		Interval: cos.Duration(10 * time.Second),
		Name:     "heartbeat",
		Factor:   3,
	},
	RetryFactor: 4,
}

var DefaultLog = aiscmn.LogConf{
	Level:     "3",
	MaxSize:   cos.SizeIEC(4 * cos.MiB),
	MaxTotal:  cos.SizeIEC(128 * cos.MiB),
	FlushTime: cos.Duration(time.Minute),
	StatsTime: cos.Duration(3 * time.Minute),
}

var DefaultSpace = aiscmn.SpaceConf{
	CleanupWM: 65,
	LowWM:     75,
	HighWM:    90,
	OOS:       95,
}

var DefaultMemsys = aiscmn.MemsysConf{
	MinFree:        cos.SizeIEC(2 * cos.GiB),
	DefaultBufSize: cos.SizeIEC(32 * cos.KiB),
	SizeToGC:       cos.SizeIEC(2 * cos.GiB),
	HousekeepTime:  cos.Duration(90 * time.Second),
}

var DefaultLRU = aiscmn.LRUConf{
	Enabled:         false,
	DontEvictTime:   cos.Duration(120 * time.Minute),
	CapacityUpdTime: cos.Duration(10 * time.Minute),
}

var DefaultMirror = aiscmn.MirrorConf{
	Enabled: false,
	Copies:  2,
	Burst:   512,
}

var DefaultPeriodic = aiscmn.PeriodConf{
	StatsTime:     cos.Duration(10 * time.Second),
	NotifTime:     cos.Duration(30 * time.Second),
	RetrySyncTime: cos.Duration(2 * time.Second),
}

var DefaultRebalance = aiscmn.RebalanceConf{
	Enabled:       true,
	Compression:   aisapc.CompressNever,
	DestRetryTime: cos.Duration(2 * time.Minute),
	SbundleMult:   2,
}

var DefaultTimeout = aiscmn.TimeoutConf{
	CplaneOperation: cos.Duration(2 * time.Second),
	MaxKeepalive:    cos.Duration(4 * time.Second),
	MaxHostBusy:     cos.Duration(20 * time.Second),
	Startup:         cos.Duration(time.Minute),
	SendFile:        cos.Duration(5 * time.Minute),
}

var DefaultVersioning = aiscmn.VersionConf{
	Enabled:         true,
	ValidateWarmGet: false,
}
