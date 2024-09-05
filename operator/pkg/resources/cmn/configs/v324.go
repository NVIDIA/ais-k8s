// Package configs provides AIS cluster config types and defaults
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package configs

import aiscmn "github.com/NVIDIA/aistore/cmn"

type V324ClusterConfig struct {
	BaseClusterConfig
}

var V324AISConf = V324ClusterConfig{
	BaseClusterConfig: BaseClusterConfig{
		ClusterConfig: aiscmn.ClusterConfig{
			Auth:       DefaultAuth,
			Cksum:      DefaultCksum,
			Client:     DefaultClientConf,
			Transport:  DefaultTransport,
			TCB:        DefaultTCB,
			Disk:       DefaultDisk,
			Net:        DefaultNet,
			FSHC:       DefaultFSHC,
			Dsort:      DefaultDsort,
			Downloader: DefaultDownloader,
			EC:         DefaultEC,
			Keepalive:  DefaultKeepalive,
			Log:        DefaultLog,
			Space:      DefaultSpace,
			Memsys:     DefaultMemsys,
			LRU:        DefaultLRU,
			Mirror:     DefaultMirror,
			Periodic:   DefaultPeriodic,
			Rebalance:  DefaultRebalance,
			Timeout:    DefaultTimeout,
			Versioning: DefaultVersioning,
		},
	},
}
