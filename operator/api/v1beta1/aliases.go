/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Code generated for legacy API source compatibility; DO NOT EDIT.

// Package v1beta1 is a compatibility shim for the AIStore v1beta1 API types.
//
// Deprecated: import github.com/ais-operator/api/aistore/v1beta1 instead.
// This package re-exports the types moved under api/aistore/v1beta1 so existing
// external importers continue to compile. It will be removed in a future release.
package v1beta1

import (
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = aisv1.GroupVersion

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = aisv1.SchemeBuilder

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = aisv1.AddToScheme
)

// Type aliases for the AIStore API types formerly defined in this package.
type (
	AIStore                       = aisv1.AIStore
	AIStoreList                   = aisv1.AIStoreList
	AIStoreSpec                   = aisv1.AIStoreSpec
	AIStoreStatus                 = aisv1.AIStoreStatus
	AdaptiveToUpdate              = aisv1.AdaptiveToUpdate
	AdminClientSpec               = aisv1.AdminClientSpec
	ArchConfToUpdate              = aisv1.ArchConfToUpdate
	AuthConfToUpdate              = aisv1.AuthConfToUpdate
	AuthServerLoginConf           = aisv1.AuthServerLoginConf
	AuthSignatureConfToUpdate     = aisv1.AuthSignatureConfToUpdate
	AuthSpec                      = aisv1.AuthSpec
	AuthTLSConfig                 = aisv1.AuthTLSConfig
	AutoScaleConf                 = aisv1.AutoScaleConf
	AutoScaleStatus               = aisv1.AutoScaleStatus
	BurstyToUpdate                = aisv1.BurstyToUpdate
	CAConfigMapRef                = aisv1.CAConfigMapRef
	CertIssuerRef                 = aisv1.CertIssuerRef
	ChunksConfToUpdate            = aisv1.ChunksConfToUpdate
	CksumConfToUpdate             = aisv1.CksumConfToUpdate
	ClientConfToUpdate            = aisv1.ClientConfToUpdate
	ClusterConditionReason        = aisv1.ClusterConditionReason
	ClusterConditionType          = aisv1.ClusterConditionType
	ClusterKeyConfToUpdate        = aisv1.ClusterKeyConfToUpdate
	ClusterState                  = aisv1.ClusterState
	ConfigToUpdate                = aisv1.ConfigToUpdate
	DSortConfToUpdate             = aisv1.DSortConfToUpdate
	DaemonSpec                    = aisv1.DaemonSpec
	DiskConfToUpdate              = aisv1.DiskConfToUpdate
	DownloaderConfToUpdate        = aisv1.DownloaderConfToUpdate
	Duration                      = aisv1.Duration
	ECConfToUpdate                = aisv1.ECConfToUpdate
	Empty                         = aisv1.Empty
	ExternalAccessSpec            = aisv1.ExternalAccessSpec
	FSHCConfToUpdate              = aisv1.FSHCConfToUpdate
	GetBatchConfToUpdate          = aisv1.GetBatchConfToUpdate
	HTTPConfToUpdate              = aisv1.HTTPConfToUpdate
	KeepaliveConfToUpdate         = aisv1.KeepaliveConfToUpdate
	KeepaliveTrackerConfToUpdate  = aisv1.KeepaliveTrackerConfToUpdate
	LRUConfToUpdate               = aisv1.LRUConfToUpdate
	LogConfToUpdate               = aisv1.LogConfToUpdate
	LogSidecarSpec                = aisv1.LogSidecarSpec
	MemsysConfToUpdate            = aisv1.MemsysConfToUpdate
	MirrorConfToUpdate            = aisv1.MirrorConfToUpdate
	Mount                         = aisv1.Mount
	NetConfToUpdate               = aisv1.NetConfToUpdate
	OIDCConfToUpdate              = aisv1.OIDCConfToUpdate
	PDBSpec                       = aisv1.PDBSpec
	PeriodConfToUpdate            = aisv1.PeriodConfToUpdate
	ProbeConfSpec                 = aisv1.ProbeConfSpec
	ProbeSpec                     = aisv1.ProbeSpec
	ProxyConfToUpdate             = aisv1.ProxyConfToUpdate
	PubNetDNSMode                 = aisv1.PubNetDNSMode
	RateLimitBaseToUpdate         = aisv1.RateLimitBaseToUpdate
	RateLimitConfToUpdate         = aisv1.RateLimitConfToUpdate
	RebalanceConfToUpdate         = aisv1.RebalanceConfToUpdate
	RequiredClaimsConfToUpdate    = aisv1.RequiredClaimsConfToUpdate
	ResilverConfToUpdate          = aisv1.ResilverConfToUpdate
	ScaleDownMode                 = aisv1.ScaleDownMode
	ServiceSpec                   = aisv1.ServiceSpec
	SizeIEC                       = aisv1.SizeIEC
	SpaceConfToUpdate             = aisv1.SpaceConfToUpdate
	StateEmptyDirConfig           = aisv1.StateEmptyDirConfig
	StateHostPathConfig           = aisv1.StateHostPathConfig
	StatePVCConfig                = aisv1.StatePVCConfig
	StateStorage                  = aisv1.StateStorage
	TCBConfToUpdate               = aisv1.TCBConfToUpdate
	TCOConfToUpdate               = aisv1.TCOConfToUpdate
	TLSCertificateConfig          = aisv1.TLSCertificateConfig
	TLSCertificateMode            = aisv1.TLSCertificateMode
	TLSSpec                       = aisv1.TLSSpec
	TargetSpec                    = aisv1.TargetSpec
	TimeoutConfToUpdate           = aisv1.TimeoutConfToUpdate
	TokenExchangeAuth             = aisv1.TokenExchangeAuth
	TraceExporterAuthConfToUpdate = aisv1.TraceExporterAuthConfToUpdate
	TracingConfToUpdate           = aisv1.TracingConfToUpdate
	TransportConfToUpdate         = aisv1.TransportConfToUpdate
	UsernamePasswordAuth          = aisv1.UsernamePasswordAuth
	VersionConfToUpdate           = aisv1.VersionConfToUpdate
	WritePolicyConfToUpdate       = aisv1.WritePolicyConfToUpdate
	XactConfToUpdate              = aisv1.XactConfToUpdate
)

// Constants formerly defined in this package.
const (
	ReasonUpgrading               = aisv1.ReasonUpgrading
	ReasonScaling                 = aisv1.ReasonScaling
	ReasonShutdown                = aisv1.ReasonShutdown
	ConditionInitialized          = aisv1.ConditionInitialized
	ConditionCreated              = aisv1.ConditionCreated
	ConditionReady                = aisv1.ConditionReady
	ConditionReadyRebalance       = aisv1.ConditionReadyRebalance
	ClusterInitialized            = aisv1.ClusterInitialized
	ClusterCreated                = aisv1.ClusterCreated
	ClusterReady                  = aisv1.ClusterReady
	ClusterInitializingLBService  = aisv1.ClusterInitializingLBService
	ClusterPendingLBService       = aisv1.ClusterPendingLBService
	ClusterUpgrading              = aisv1.ClusterUpgrading
	ClusterShuttingDown           = aisv1.ClusterShuttingDown
	ClusterShutdown               = aisv1.ClusterShutdown
	ClusterDecommissioning        = aisv1.ClusterDecommissioning
	ClusterCleanup                = aisv1.ClusterCleanup
	HostCleanup                   = aisv1.HostCleanup
	ClusterFinalized              = aisv1.ClusterFinalized
	PubNetDNSModeIP               = aisv1.PubNetDNSModeIP
	PubNetDNSModeNode             = aisv1.PubNetDNSModeNode
	PubNetDNSModePod              = aisv1.PubNetDNSModePod
	ScaleDownModeDecommission     = aisv1.ScaleDownModeDecommission
	ScaleDownModeRetain           = aisv1.ScaleDownModeRetain
	ScaleDownModeSafeDecommission = aisv1.ScaleDownModeSafeDecommission
	TLSCertificateModeSecret      = aisv1.TLSCertificateModeSecret
	TLSCertificateModeCSI         = aisv1.TLSCertificateModeCSI
)
