// Package configs provides AIS cluster config types and defaults
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package configs

import aiscmn "github.com/NVIDIA/aistore/cmn"

// V323FSHCConf See aistore cmn/config.go FSHCConf
type v323FSHCConf struct {
	TestFileCount int  `json:"test_files"`
	HardErrs      int  `json:"error_limit"`
	Enabled       bool `json:"enabled"`
}

var v323FSHC = v323FSHCConf{
	TestFileCount: 4,
	HardErrs:      2,
	Enabled:       true,
}

// Based on the aistore cmn/config.go ClusterConfig in release 3.23
type V323ClusterConfig struct {
	BaseClusterConfig
	// Override with 3.23 specific
	FSHC v323FSHCConf `json:"fshc"`
}

var V323AISConf = V323ClusterConfig{
	BaseClusterConfig: BaseClusterConfig{
		ClusterConfig: aiscmn.ClusterConfig{
			Auth:       DefaultAuth,
			Cksum:      DefaultCksum,
			Client:     DefaultClientConf,
			Transport:  DefaultTransport,
			TCB:        DefaultTCB,
			Disk:       DefaultDisk,
			Net:        DefaultNet,
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
	FSHC: v323FSHC,
}
