/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

// Reasons to be used by event recorder.
const (
	EventReasonReady            = "Ready"
	EventReasonRolloutStalled   = "RolloutStalled"
	EventReasonCleanupCompleted = "CleanupCompleted"

	EventReasonConfigMapFailed        = "ConfigMapFailed"
	EventReasonPVCFailed              = "PVCFailed"
	EventReasonServicesFailed         = "ServicesFailed"
	EventReasonCertificateFailed      = "CertificateFailed"
	EventReasonDeploymentFailed       = "DeploymentFailed"
	EventReasonPVCRetentionFailed     = "PVCRetentionFailed"
	EventReasonFinalizerRemovalFailed = "FinalizerRemovalFailed"
	EventReasonFinalizerFailed        = "FinalizerFailed"
)

// Actions to be used in events.
const (
	ActionReconcile = "Reconciled"
	ActionDelete    = "Delete"
)
