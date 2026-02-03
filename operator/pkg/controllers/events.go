// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

// Reasons to be used by event recorder
const (
	EventReasonInitialized           = "Initialized"
	EventReasonFailed                = "Failed"
	EventReasonWaiting               = "Waiting"
	EventReasonCreated               = "Created"
	EventReasonReady                 = "Ready"
	EventReasonBackOff               = "BackOff"
	EventReasonShutdownCompleted     = "ShutdownCompleted"
	EventReasonDecommissionCompleted = "DecommissionCompleted"
	EventReasonDeleted               = "CRDeleted"
	EventReasonUpdated               = "CRUpdated"
)

// Actions to be used in events
const (
	ActionStartDecommission = "Decommissioning"
	ActionStartShutdown     = "StartShutdown"
	ActionFinishShutdown    = "FinishShutdown"
	ActionCreate            = "Create"
	ActionDelete            = "Delete"
	ActionReconcile         = "Reconciled"
	ActionInitProxyLB       = "InitProxyLB"
	ActionWaitForProxyLB    = "WaitingForProxyLB"
	ActionInitTargets       = "InitTargets"
	ActionInitProxies       = "InitProxies"
)
