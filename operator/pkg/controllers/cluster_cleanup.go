// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"fmt"
	"time"

	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/ais-operator/pkg/resources/target"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *AIStoreReconciler) cleanup(ctx context.Context, ais *aisv1.AIStore) (updated bool, err error) {
	var nodeNames map[string]bool
	nodeNames, err = r.client.ListNodesRunningAIS(ctx, ais)
	if err != nil {
		r.log.Error(err, "Failed to list nodes running AIS")
	}
	updated, err = cmn.AnyFunc(
		func() (bool, error) { return r.cleanupTarget(ctx, ais) },
		func() (bool, error) { return r.cleanupProxy(ctx, ais) },
		func() (bool, error) { return r.client.DeleteConfigMapIfExists(ctx, statsd.ConfigMapNSName(ais)) },
		func() (bool, error) { return r.cleanupRBAC(ctx, ais) },
		func() (bool, error) { return r.cleanupPVC(ctx, ais) },
	)
	if updated && ais.ShouldCleanupMetadata() {
		jobs, err := r.createCleanupJobs(ctx, ais, nodeNames)
		if err != nil {
			r.log.Error(err, "Failed to run manual cleanup job")
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
		defer cancel()
		if err := r.waitForJobs(timeoutCtx, jobs); err != nil {
			r.log.Error(err, "Error while waiting for cleanup jobs to complete")
		}
	}
	return updated, err
}

func (r *AIStoreReconciler) createCleanupJobs(ctx context.Context, ais *aisv1.AIStore, uniqueNodeNames map[string]bool) ([]*batchv1.Job, error) {
	if ais.Spec.StateStorageClass != nil {
		return nil, nil
	}

	r.log.Info("Running manual cleanup job")
	jobs := make([]*batchv1.Job, 0, len(uniqueNodeNames))

	for nodeName := range uniqueNodeNames {
		r.log.Info("Creating cleanup job for node", "node", nodeName)
		jobDef := cmn.NewCleanupJob(ais, nodeName)
		if err := r.client.Create(ctx, jobDef); err != nil {
			return jobs, err
		}
		jobs = append(jobs, jobDef)
	}
	return jobs, nil
}

func (r *AIStoreReconciler) waitForJobs(ctx context.Context, jobs []*batchv1.Job) error {
	for _, job := range jobs {
		r.log.Info("Waiting for job to complete", "jobName", job.Name)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				jobStatus := &batchv1.Job{}
				if err := r.client.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, jobStatus); err != nil {
					r.log.Error(err, "Failed to get job status", "jobName", job.Name)
					return err
				}
				if jobStatus.Status.Succeeded > 0 {
					r.log.Info("Job completed successfully", "jobName", job.Name)
					break
				}
				if jobStatus.Status.Failed > 0 {
					err := fmt.Errorf("job %s failed", job.Name)
					r.log.Error(err, "Job failed", "jobName", job.Name)
					return err
				}
				time.Sleep(2 * time.Second)
			}
		}
	}
	return nil
}

func (r *AIStoreReconciler) cleanupPVC(ctx context.Context, ais *aisv1.AIStore) (bool, error) {
	if !ais.ShouldCleanupMetadata() {
		return false, nil
	}
	if ais.Spec.CleanupData != nil && *ais.Spec.CleanupData {
		return r.deleteAllPVCs(ctx, ais)
	}
	if ais.Spec.StateStorageClass != nil {
		return r.deleteStatePVCs(ctx, ais)
	}
	return false, nil
}

func (r *AIStoreReconciler) deleteAllPVCs(ctx context.Context, ais *aisv1.AIStore) (bool, error) {
	r.log.Info("Cleaning up all target PVCs")
	updated, err := r.client.DeletePVCs(ctx, ais.Namespace, target.PodLabels(ais), nil)
	if err != nil {
		return updated, err
	}
	r.log.Info("Cleaning up all proxy PVCs")
	return r.client.DeletePVCs(ctx, ais.Namespace, proxy.PodLabels(ais), nil)
}

// Cleans up only dynamically created volumes by adding a filter by the defined state storage class
func (r *AIStoreReconciler) deleteStatePVCs(ctx context.Context, ais *aisv1.AIStore) (bool, error) {
	r.log.Info("Cleaning up dynamic target PVCs")
	updated, err := r.client.DeletePVCs(ctx, ais.Namespace, target.PodLabels(ais), ais.Spec.StateStorageClass)
	if err != nil {
		return updated, err
	}
	r.log.Info("Cleaning up dynamic proxy PVCs")
	return r.client.DeletePVCs(ctx, ais.Namespace, proxy.PodLabels(ais), ais.Spec.StateStorageClass)
}

func (r *AIStoreReconciler) cleanupRBAC(ctx context.Context, ais *aisv1.AIStore) (anyUpdated bool, err error) {
	return cmn.AnyFunc(
		func() (bool, error) {
			crb := cmn.NewAISRBACClusterRoleBinding(ais)
			return r.client.DeleteResourceIfExists(ctx, crb)
		},
		func() (bool, error) {
			cluRole := cmn.NewAISRBACClusterRole(ais)
			return r.client.DeleteResourceIfExists(ctx, cluRole)
		},
		func() (bool, error) {
			rb := cmn.NewAISRBACRoleBinding(ais)
			return r.client.DeleteResourceIfExists(ctx, rb)
		},
		func() (bool, error) {
			role := cmn.NewAISRBACRole(ais)
			return r.client.DeleteResourceIfExists(ctx, role)
		},
		func() (bool, error) {
			sa := cmn.NewAISServiceAccount(ais)
			return r.client.DeleteResourceIfExists(ctx, sa)
		},
	)
}
