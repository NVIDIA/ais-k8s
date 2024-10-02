// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"strings"
	"time"

	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/ais-operator/pkg/resources/target"
	batchv1 "k8s.io/api/batch/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *AIStoreReconciler) cleanup(ctx context.Context, ais *aisv1.AIStore) (updated bool, err error) {
	nodeNames, err := r.k8sClient.ListNodesRunningAIS(ctx, ais)
	if err != nil {
		r.log.Error(err, "Failed to list nodes running AIS")
	}
	updated, err = cmn.AnyFunc(
		func() (bool, error) { return r.cleanupTarget(ctx, ais) },
		func() (bool, error) { return r.cleanupProxy(ctx, ais) },
		func() (bool, error) { return r.k8sClient.DeleteConfigMapIfExists(ctx, statsd.ConfigMapNSName(ais)) },
		func() (bool, error) { return r.cleanupRBAC(ctx, ais) },
		func() (bool, error) { return r.cleanupPVC(ctx, ais) },
	)
	if updated && ais.ShouldCleanupMetadata() {
		err = r.createCleanupJobs(ctx, ais, nodeNames)
		if err != nil {
			return
		}
		err = r.updateStatusWithState(ctx, ais, aisv1.HostCleanup)
		if err != nil {
			return
		}
	}
	return
}

func (r *AIStoreReconciler) createCleanupJobs(ctx context.Context, ais *aisv1.AIStore, nodes []string) error {
	if ais.Spec.StateStorageClass != nil {
		return nil
	}
	logger := logf.FromContext(ctx)
	logger.Info("Creating manual cleanup jobs", "nodes", nodes)
	for _, nodeName := range nodes {
		jobDef := cmn.NewCleanupJob(ais, nodeName)
		if err := r.k8sClient.Create(ctx, jobDef); err != nil {
			logger.Error(err, "Failed to create cleanup job", "name", jobDef.Name, "node", nodeName)
			return err
		}
	}
	return nil
}

func (r *AIStoreReconciler) listCleanupJobs(ctx context.Context, namespace string) (*batchv1.JobList, error) {
	var cleanupJobs batchv1.JobList
	jobs, err := r.k8sClient.ListJobsInNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}
	for i := range jobs.Items {
		job := &jobs.Items[i]
		if strings.HasPrefix(job.Name, cmn.CleanupPrefix) {
			cleanupJobs.Items = append(cleanupJobs.Items, *job)
		}
	}
	return &cleanupJobs, nil
}

func (r *AIStoreReconciler) deleteFinishedJobs(ctx context.Context, jobs *batchv1.JobList) (*batchv1.JobList, error) {
	remainingJobs := &batchv1.JobList{}
	logger := logf.FromContext(ctx)

	for i := range jobs.Items {
		job := &jobs.Items[i]
		// Job succeeded, delete it
		if job.Status.Succeeded > 0 {
			_, err := r.k8sClient.DeleteResourceIfExists(ctx, job)
			if err != nil {
				logger.Error(err, "Failed to delete successful job", "name", job.Name)
				return nil, err
			}
			logger.Info("Deleted successful cleanup job", "name", job.Name)
		}
		// Job has been stuck too long, delete it
		if time.Since(job.CreationTimestamp.Time) > 2*time.Minute {
			_, err := r.k8sClient.DeleteResourceIfExists(ctx, job)
			if err != nil {
				logger.Error(err, "Failed to delete expired job", "name", job.Name)
				return nil, err
			}
			logger.Info("Aborted expired job", "job", job.Name)
		}
		remainingJobs.Items = append(remainingJobs.Items, *job)
	}
	return remainingJobs, nil
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
	updated, err := r.k8sClient.DeletePVCs(ctx, ais.Namespace, target.PodLabels(ais), nil)
	if err != nil {
		return updated, err
	}
	r.log.Info("Cleaning up all proxy PVCs")
	return r.k8sClient.DeletePVCs(ctx, ais.Namespace, proxy.PodLabels(ais), nil)
}

// Cleans up only dynamically created volumes by adding a filter by the defined state storage class
func (r *AIStoreReconciler) deleteStatePVCs(ctx context.Context, ais *aisv1.AIStore) (bool, error) {
	r.log.Info("Cleaning up dynamic target PVCs")
	updated, err := r.k8sClient.DeletePVCs(ctx, ais.Namespace, target.PodLabels(ais), ais.Spec.StateStorageClass)
	if err != nil {
		return updated, err
	}
	r.log.Info("Cleaning up dynamic proxy PVCs")
	return r.k8sClient.DeletePVCs(ctx, ais.Namespace, proxy.PodLabels(ais), ais.Spec.StateStorageClass)
}

func (r *AIStoreReconciler) cleanupRBAC(ctx context.Context, ais *aisv1.AIStore) (anyUpdated bool, err error) {
	return cmn.AnyFunc(
		func() (bool, error) {
			crb := cmn.NewAISRBACClusterRoleBinding(ais)
			return r.k8sClient.DeleteResourceIfExists(ctx, crb)
		},
		func() (bool, error) {
			cluRole := cmn.NewAISRBACClusterRole(ais)
			return r.k8sClient.DeleteResourceIfExists(ctx, cluRole)
		},
		func() (bool, error) {
			rb := cmn.NewAISRBACRoleBinding(ais)
			return r.k8sClient.DeleteResourceIfExists(ctx, rb)
		},
		func() (bool, error) {
			role := cmn.NewAISRBACRole(ais)
			return r.k8sClient.DeleteResourceIfExists(ctx, role)
		},
		func() (bool, error) {
			sa := cmn.NewAISServiceAccount(ais)
			return r.k8sClient.DeleteResourceIfExists(ctx, sa)
		},
	)
}
