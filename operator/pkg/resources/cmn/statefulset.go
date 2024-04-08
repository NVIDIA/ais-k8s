// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

func ParseAnnotations(ais *aisv1.AIStore) map[string]string {
	if ais.Spec.NetAttachment != nil {
		return map[string]string{
			nadv1.NetworkAttachmentAnnot: *ais.Spec.NetAttachment,
		}
	}
	return nil
}
