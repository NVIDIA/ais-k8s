// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"time"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
)

var defaultAISConf = aiscmn.ClusterConfig{
	Auth: aiscmn.AuthConf{
		Enabled: false,
	},
	Cksum: aiscmn.CksumConf{
		Type:            cos.ChecksumXXHash,
		ValidateColdGet: true,
	},
	Client: aiscmn.ClientConf{
		Timeout:        cos.Duration(time.Minute),
		TimeoutLong:    cos.Duration(30 * time.Minute),
		ListObjTimeout: cos.Duration(10 * time.Minute),
	},
	Transport: aiscmn.TransportConf{
		MaxHeaderSize:   4096,
		Burst:           32,
		IdleTeardown:    cos.Duration(4 * time.Second),
		QuiesceTime:     cos.Duration(10 * time.Second),
		LZ4BlockMaxSize: cos.Size(256 * cos.KiB),
	},
	TCB: aiscmn.TCBConf{
		Compression: aisapc.CompressNever,
		SbundleMult: 2,
	},
	Disk: aiscmn.DiskConf{
		DiskUtilLowWM:   20,
		DiskUtilHighWM:  80,
		DiskUtilMaxWM:   95,
		IostatTimeLong:  cos.Duration(2 * time.Second),
		IostatTimeShort: cos.Duration(100 * time.Millisecond),
	},
	// Network hostnames are substituted in InitContainer.
	Net: aiscmn.NetConf{
		L4: aiscmn.L4Conf{
			Proto: "tcp",
		},
		HTTP: aiscmn.HTTPConf{
			UseHTTPS: false,
			Chunked:  true,
		},
	},
	DSort: aiscmn.DSortConf{
		Compression:         aisapc.CompressNever,
		DuplicatedRecords:   aiscmn.IgnoreReaction,
		MissingShards:       aiscmn.IgnoreReaction,
		EKMMalformedLine:    aisapc.Abort,
		EKMMissingKey:       aisapc.Abort,
		DefaultMaxMemUsage:  "80%",
		DSorterMemThreshold: "100GB",
		CallTimeout:         cos.Duration(10 * time.Minute),
	},
	Downloader: aiscmn.DownloaderConf{
		Timeout: cos.Duration(time.Hour),
	},
	EC: aiscmn.ECConf{
		Enabled:      false,
		ObjSizeLimit: 262144,
		DataSlices:   2,
		ParitySlices: 2,
		Compression:  aisapc.CompressNever,
	},
	FSHC: aiscmn.FSHCConf{
		Enabled:       true,
		TestFileCount: 4,
		ErrorLimit:    2,
	},
	Keepalive: aiscmn.KeepaliveConf{
		Proxy: aiscmn.KeepaliveTrackerConf{
			Interval: cos.Duration(10 * time.Second),
			Name:     aiscmn.KeepaliveHeartbeatType,
			Factor:   3,
		},
		Target: aiscmn.KeepaliveTrackerConf{
			Interval: cos.Duration(10 * time.Second),
			Name:     aiscmn.KeepaliveHeartbeatType,
			Factor:   3,
		},
		RetryFactor: 5,
	},
	Log: aiscmn.LogConf{
		Level:    "3",
		MaxSize:  cos.Size(4 * cos.MiB),
		MaxTotal: cos.Size(64 * cos.MiB),
	},
	Space: aiscmn.SpaceConf{
		CleanupWM: 65,
		LowWM:     75,
		HighWM:    90,
		OOS:       95,
	},
	Memsys: aiscmn.MemsysConf{
		MinFree:        cos.Size(2 * cos.GiB),
		DefaultBufSize: cos.Size(32 * cos.KiB),
		SizeToGC:       cos.Size(2 * cos.GiB),
		HousekeepTime:  cos.Duration(90 * time.Second),
	},
	LRU: aiscmn.LRUConf{
		DontEvictTime:   cos.Duration(120 * time.Minute),
		CapacityUpdTime: cos.Duration(10 * time.Minute),
		Enabled:         false,
	},
	Mirror: aiscmn.MirrorConf{
		Copies:  2,
		Burst:   512,
		Enabled: true,
	},
	Periodic: aiscmn.PeriodConf{
		StatsTime:     cos.Duration(10 * time.Second),
		NotifTime:     cos.Duration(30 * time.Second),
		RetrySyncTime: cos.Duration(2 * time.Second),
	},
	Rebalance: aiscmn.RebalanceConf{
		Enabled:       true,
		Compression:   aisapc.CompressNever,
		DestRetryTime: cos.Duration(2 * time.Minute),
		SbundleMult:   2,
	},
	Timeout: aiscmn.TimeoutConf{
		CplaneOperation: cos.Duration(2 * time.Second),
		MaxKeepalive:    cos.Duration(4 * time.Second),
		MaxHostBusy:     cos.Duration(20 * time.Minute),
		Startup:         cos.Duration(time.Minute),
		SendFile:        cos.Duration(5 * time.Minute),
	},
	Versioning: aiscmn.VersionConf{
		Enabled:         true,
		ValidateWarmGet: false,
	},
}

func DefaultAISConf(ais *aisv1.AIStore) aiscmn.ClusterConfig {
	conf := defaultAISConf
	proxyPort := ais.Spec.ProxySpec.ServicePort.String()
	proxyURL := "http://" + ais.Name + "-proxy:" + proxyPort
	conf.Proxy = aiscmn.ProxyConf{
		PrimaryURL:   proxyURL,
		OriginalURL:  proxyURL,
		DiscoveryURL: proxyURL,
	}
	return conf
}

func LocalConfTemplate(sp aisv1.ServiceSpec, mounts []aisv1.Mount) aiscmn.LocalConfig {
	localConf := aiscmn.LocalConfig{
		ConfigDir: "/etc/ais",
		LogDir:    "/var/log/ais",
		HostNet: aiscmn.LocalNetConfig{
			Hostname:             "${AIS_PUBLIC_HOSTNAME}",
			HostnameIntraControl: "${AIS_INTRA_HOSTNAME}",
			HostnameIntraData:    "${AIS_DATA_HOSTNAME}",
			Port:                 sp.PublicPort.IntValue(),
			PortIntraControl:     sp.IntraControlPort.IntValue(),
			PortIntraData:        sp.IntraDataPort.IntValue(),
		},
	}
	if len(mounts) == 0 {
		return localConf
	}

	localConf.FSP.Paths = make(cos.StrSet, len(mounts))
	for _, m := range mounts {
		localConf.FSP.Paths.Add(m.Path)
	}
	return localConf
}
