// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"fmt"
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
		LZ4BlockMaxSize: cos.SizeIEC(256 * cos.KiB),
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
	Dsort: aiscmn.DsortConf{
		Compression:         aisapc.CompressNever,
		DuplicatedRecords:   aiscmn.IgnoreReaction,
		MissingShards:       aiscmn.IgnoreReaction,
		EKMMalformedLine:    aisapc.Abort,
		EKMMissingKey:       aisapc.Abort,
		DefaultMaxMemUsage:  "80%",
		DsorterMemThreshold: "100GB",
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
			Name:     "heartbeat",
			Factor:   3,
		},
		Target: aiscmn.KeepaliveTrackerConf{
			Interval: cos.Duration(10 * time.Second),
			Name:     "heartbeat",
			Factor:   3,
		},
		RetryFactor: 5,
	},
	Log: aiscmn.LogConf{
		Level:    "3",
		MaxSize:  cos.SizeIEC(4 * cos.MiB),
		MaxTotal: cos.SizeIEC(64 * cos.MiB),
	},
	Space: aiscmn.SpaceConf{
		CleanupWM: 65,
		LowWM:     75,
		HighWM:    90,
		OOS:       95,
	},
	Memsys: aiscmn.MemsysConf{
		MinFree:        cos.SizeIEC(2 * cos.GiB),
		DefaultBufSize: cos.SizeIEC(32 * cos.KiB),
		SizeToGC:       cos.SizeIEC(2 * cos.GiB),
		HousekeepTime:  cos.Duration(90 * time.Second),
	},
	LRU: aiscmn.LRUConf{
		Enabled:         false,
		DontEvictTime:   cos.Duration(120 * time.Minute),
		CapacityUpdTime: cos.Duration(10 * time.Minute),
	},
	Mirror: aiscmn.MirrorConf{
		Enabled: false,
		Copies:  2,
		Burst:   512,
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
		MaxHostBusy:     cos.Duration(20 * time.Second),
		Startup:         cos.Duration(time.Minute),
		SendFile:        cos.Duration(5 * time.Minute),
	},
	Versioning: aiscmn.VersionConf{
		Enabled:         true,
		ValidateWarmGet: false,
	},
}

func convertConfig(cfg *aisv1.ConfigToUpdate) (toUpdate *aiscmn.ConfigToSet, err error) {
	toUpdate = &aiscmn.ConfigToSet{}
	err = cos.MorphMarshal(cfg, toUpdate)
	return toUpdate, err
}

func DefaultAISConf(ais *aisv1.AIStore) aiscmn.ClusterConfig {
	var scheme string
	conf := defaultAISConf
	if ais.Spec.TLSSecretName == nil {
		scheme = "http"
	} else {
		scheme = "https"
	}
	primaryProxy := ais.DefaultPrimaryName()
	domain := ais.GetClusterDomain()
	svcName := ais.ProxyStatefulSetName()
	intraCtrlPort := ais.Spec.ProxySpec.IntraControlPort.String()
	// Example: http://ais-proxy-0.ais-proxy.ais.svc.cluster.local:51080
	proxyURL := fmt.Sprintf("%s://%s.%s.%s.svc.%s:%s", scheme, primaryProxy, svcName, ais.Namespace, domain, intraCtrlPort)

	conf.Proxy = aiscmn.ProxyConf{
		PrimaryURL:   proxyURL,
		OriginalURL:  proxyURL,
		DiscoveryURL: proxyURL,
	}
	return conf
}
