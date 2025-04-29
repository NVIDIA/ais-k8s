// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"context"
	"fmt"
	"io"
	"os"

	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
)

// PrintLogs prints all logs from proxy and target pods in the given AIStore
// cluster to stdout.
func PrintLogs(ctx context.Context, cluster *aisv1.AIStore, client *aisclient.K8sClient) (err error) {
	cs, err := NewClientset()
	if err != nil {
		return fmt.Errorf("error creating clientset: %v", err)
	}
	clusterSelector := map[string]string{"app.kubernetes.io/name": cluster.Name}
	podList, err := client.ListPods(ctx, cluster, clusterSelector)
	if err != nil {
		return fmt.Errorf("error listing pods for cluster %s: %v", cluster.Name, err)
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		opts := &corev1.PodLogOptions{Container: "ais-logs"}
		req := cs.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts)
		stream, err := req.Stream(ctx)
		if err != nil {
			return fmt.Errorf("error opening log stream: %v", err)
		}
		defer stream.Close()
		fmt.Printf("Logs for pod %s in cluster %s:\n", pod.Name, cluster.Name)
		if _, err := io.Copy(os.Stdout, stream); err != nil {
			return fmt.Errorf("error printing logs for pod %s in cluster %s: %v", pod.Name, cluster.Name, err)
		}
	}
	return nil
}
