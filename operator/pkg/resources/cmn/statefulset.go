// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"fmt"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

func ProxyStatefulSetName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aisapc.Proxy
}

// DefaultPrimaryName returns name of pod used as default Primary
func DefaultPrimaryProxyName(ais *aisv1.AIStore) string {
	return ProxyStatefulSetName(ais) + "-0"
}

// DefaultPrimaryProxyURL constructs the URL for the default primary proxy using the specified port.
func DefaultPrimaryProxyURL(ais *aisv1.AIStore, port string) string {
	return fmt.Sprintf("%s.%s.%s.svc.%s:%s",
		DefaultPrimaryProxyName(ais), ProxyStatefulSetName(ais), ais.Namespace, ais.GetClusterDomain(), port)
}

func ParseAnnotations(ais *aisv1.AIStore) map[string]string {
	if ais.Spec.NetAttachment != nil {
		return map[string]string{
			nadv1.NetworkAttachmentAnnot: *ais.Spec.NetAttachment,
		}
	}
	return nil
}
