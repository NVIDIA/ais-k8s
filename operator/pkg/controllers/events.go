// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package controllers

// Reason's to be used by event recorder
const (
	EventReasonInitialized = "Initialized"
	EventReasonFailed      = "Failed"
	EventReasonWaiting     = "Waiting"
	EventReasonCreated     = "Created"
	EventReasonReady       = "Ready"
	EventReasonBackOff     = "BackOff"
)
