// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"time"

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
		TimeoutStr:     (120 * time.Second).String(),
		TimeoutLongStr: (30 * time.Minute).String(),
		ListObjectsStr: (10 * time.Minute).String(),
	},
	Compression: aiscmn.CompressionConf{
		BlockMaxSize: 262144,
		Checksum:     false,
	},
	Disk: aiscmn.DiskConf{
		DiskUtilLowWM:      20,
		DiskUtilHighWM:     80,
		DiskUtilMaxWM:      95,
		IostatTimeLongStr:  (2 * time.Second).String(),
		IostatTimeShortStr: (100 * time.Millisecond).String(),
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
		Compression:         aiscmn.CompressNever,
		DuplicatedRecords:   aiscmn.IgnoreReaction,
		MissingShards:       aiscmn.IgnoreReaction,
		EKMMalformedLine:    aiscmn.Abort,
		EKMMissingKey:       aiscmn.Abort,
		DefaultMaxMemUsage:  "80%",
		DSorterMemThreshold: "100GB",
		CallTimeoutStr:      (10 * time.Minute).String(),
	},
	Downloader: aiscmn.DownloaderConf{
		TimeoutStr: time.Hour.String(),
	},
	EC: aiscmn.ECConf{
		Enabled:      false,
		ObjSizeLimit: 262144,
		DataSlices:   2,
		ParitySlices: 2,
		BatchSize:    64,
		Compression:  aiscmn.CompressNever,
	},
	FSHC: aiscmn.FSHCConf{
		Enabled:       true,
		TestFileCount: 4,
		ErrorLimit:    2,
	},
	Keepalive: aiscmn.KeepaliveConf{
		Proxy: aiscmn.KeepaliveTrackerConf{
			IntervalStr: (10 * time.Second).String(),
			Name:        aiscmn.KeepaliveHeartbeatType,
			Factor:      3,
		},
		Target: aiscmn.KeepaliveTrackerConf{
			IntervalStr: (10 * time.Second).String(),
			Name:        aiscmn.KeepaliveHeartbeatType,
			Factor:      3,
		},
		RetryFactor:   5,
		TimeoutFactor: 3,
	},
	Log: aiscmn.LogConf{
		Level:    "3",
		MaxSize:  4194304,
		MaxTotal: 67108864,
	},
	LRU: aiscmn.LRUConf{
		LowWM:              75,
		HighWM:             90,
		OOS:                95,
		DontEvictTimeStr:   (120 * time.Minute).String(),
		CapacityUpdTimeStr: (10 * time.Minute).String(),
		Enabled:            false,
	},
	Mirror: aiscmn.MirrorConf{
		Copies:      2,
		Burst:       512,
		UtilThresh:  0,
		OptimizePUT: false,
		Enabled:     true,
	},
	Periodic: aiscmn.PeriodConf{
		StatsTimeStr:     (10 * time.Second).String(),
		NotifTimeStr:     (30 * time.Second).String(),
		RetrySyncTimeStr: (2 * time.Second).String(),
	},
	Rebalance: aiscmn.RebalanceConf{
		Enabled:          true,
		Compression:      aiscmn.CompressNever,
		DestRetryTimeStr: "2m",
		QuiesceStr:       "20s",
		Multiplier:       2,
	},
	Timeout: aiscmn.TimeoutConf{
		CplaneOperationStr: "2s",
		MaxKeepaliveStr:    "4s",
		MaxHostBusyStr:     "20s",
		StartupStr:         "1m",
		SendFileStr:        "5m",
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
			PortStr:              sp.PublicPort.String(),
			PortIntraControlStr:  sp.IntraControlPort.String(),
			PortIntraDataStr:     sp.IntraDataPort.String(),
		},
	}
	if len(mounts) == 0 {
		return localConf
	}

	localConf.FSpaths.Paths = make(cos.StringSet, len(mounts))
	for _, m := range mounts {
		localConf.FSpaths.Paths.Add(m.Path)
	}
	return localConf
}
