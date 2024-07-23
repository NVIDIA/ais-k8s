// Package client contains wrapper for k8s client
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package client

import (
	"context"
	"fmt"
	"time"

	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/target"
	apiv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type (
	K8sClient struct {
		client client.Client
		scheme *runtime.Scheme
	}
)

func NewClient(c client.Client, s *runtime.Scheme) *K8sClient {
	return &K8sClient{
		client: c,
		scheme: s,
	}
}

func NewClientFromMgr(mgr manager.Manager) *K8sClient {
	return NewClient(mgr.GetClient(), mgr.GetScheme())
}

/////////////////////////////////////////
//             Get resources           //
/////////////////////////////////////////

func (c *K8sClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.client.List(ctx, list, opts...)
}

func (c *K8sClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return c.client.Get(ctx, key, obj)
}

func (c *K8sClient) GetAIStoreCR(ctx context.Context, name types.NamespacedName) (*aisv1.AIStore, error) {
	aistore := &aisv1.AIStore{}
	err := c.client.Get(ctx, name, aistore)
	return aistore, err
}

func (c *K8sClient) ListAIStoreCR(ctx context.Context, namespace string) (*aisv1.AIStoreList, error) {
	list := &aisv1.AIStoreList{}
	err := c.client.List(ctx, list, client.InNamespace(namespace))
	return list, err
}

func (c *K8sClient) ListProxyPods(ctx context.Context, ais *aisv1.AIStore) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	err := c.client.List(ctx, podList, client.InNamespace(ais.Namespace), client.MatchingLabels(proxy.PodLabels(ais)))
	return podList, err
}

func (c *K8sClient) ListTargetPods(ctx context.Context, ais *aisv1.AIStore) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	err := c.client.List(ctx, podList, client.InNamespace(ais.Namespace), client.MatchingLabels(target.PodLabels(ais)))
	return podList, err
}

func (c *K8sClient) ListJobsInNamespace(ctx context.Context, namespace string) (*batchv1.JobList, error) {
	jobList := &batchv1.JobList{}
	err := c.client.List(ctx, jobList, client.InNamespace(namespace))
	return jobList, err
}

func (c *K8sClient) GetStatefulSet(ctx context.Context, name types.NamespacedName) (*apiv1.StatefulSet, error) {
	ss := &apiv1.StatefulSet{}
	err := c.client.Get(ctx, name, ss)
	return ss, err
}

func (c *K8sClient) StatefulSetExists(ctx context.Context, name types.NamespacedName) (exists bool, err error) {
	_, err = c.GetStatefulSet(ctx, name)
	if err == nil {
		exists = true
		return
	}
	if apierrors.IsNotFound(err) {
		err = nil
	}
	return
}

func (c *K8sClient) GetServiceByName(ctx context.Context, name types.NamespacedName) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := c.client.Get(ctx, name, svc)
	return svc, err
}

func (c *K8sClient) GetCMByName(ctx context.Context, name types.NamespacedName) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := c.client.Get(ctx, name, cm)
	return cm, err
}

func (c *K8sClient) GetPodByName(ctx context.Context, name types.NamespacedName) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	err := c.client.Get(ctx, name, pod)
	return pod, err
}

func (c *K8sClient) GetRoleByName(ctx context.Context, name types.NamespacedName) (*rbacv1.Role, error) {
	role := &rbacv1.Role{}
	err := c.client.Get(ctx, name, role)
	return role, err
}

func (c *K8sClient) Status() client.StatusWriter { return c.client.Status() }

// listPodsAndUpdateNodeNames lists pods based on the provided label selector and updates the uniqueNodeNames map with the node names of those pods
func (c *K8sClient) listPodsAndUpdateNodeNames(ctx context.Context, ais *aisv1.AIStore, labelSelector map[string]string, uniqueNodeNames map[string]bool) error {
	pods := &corev1.PodList{}
	if err := c.List(ctx, pods, client.InNamespace(ais.Namespace), client.MatchingLabels(labelSelector)); err != nil {
		return err
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		// check if the pod is running on a node (not failed or pending)
		if pod.Spec.NodeName != "" {
			uniqueNodeNames[pod.Spec.NodeName] = true
		}
	}
	return nil
}

// ListNodesMatchingSelector returns a NodeList matching the given node selector
func (c *K8sClient) ListNodesMatchingSelector(ctx context.Context, nodeSelector map[string]string) (*corev1.NodeList, error) {
	nodeList := &corev1.NodeList{}
	listOpts := &client.ListOptions{LabelSelector: labels.SelectorFromSet(nodeSelector)}
	err := c.client.List(ctx, nodeList, listOpts)
	return nodeList, err
}

// ListNodesRunningAIS returns a map of unique node names where AIS pods are running
func (c *K8sClient) ListNodesRunningAIS(ctx context.Context, ais *aisv1.AIStore) (map[string]bool, error) {
	uniqueNodeNames := make(map[string]bool)
	if err := c.listPodsAndUpdateNodeNames(ctx, ais, proxy.PodLabels(ais), uniqueNodeNames); err != nil {
		return nil, err
	}
	if err := c.listPodsAndUpdateNodeNames(ctx, ais, target.PodLabels(ais), uniqueNodeNames); err != nil {
		return nil, err
	}
	return uniqueNodeNames, nil
}

// GetStorageClasses returns a map of installed storage classes in the k8s cluster
func (c *K8sClient) GetStorageClasses(ctx context.Context) (map[string]*storagev1.StorageClass, error) {
	scList := &storagev1.StorageClassList{}
	if err := c.client.List(ctx, scList); err != nil {
		return nil, err
	}
	scMap := make(map[string]*storagev1.StorageClass, len(scList.Items))
	for i := range scList.Items {
		scMap[scList.Items[i].Name] = &scList.Items[i]
	}
	return scMap, nil
}

///////////////////////////////////////
//      create/update resources     //
//////////////////////////////////////

func (c *K8sClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return c.client.Update(ctx, obj, opts...)
}

func (c *K8sClient) UpdateIfExists(ctx context.Context, res client.Object) error {
	err := c.client.Update(ctx, res)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *K8sClient) UpdateStatefulSetReplicas(ctx context.Context, name types.NamespacedName, size int32) (updated bool, err error) {
	ss, err := c.GetStatefulSet(ctx, name)
	if err != nil {
		return
	}
	updated = *ss.Spec.Replicas != size
	if !updated {
		return
	}
	ss.Spec.Replicas = &size
	err = c.client.Update(ctx, ss)
	return
}

func (c *K8sClient) UpdateStatefulSetImage(ctx context.Context, name types.NamespacedName, idx int, newImage string) (updated bool, err error) {
	ss, err := c.GetStatefulSet(ctx, name)
	if err != nil {
		return
	}
	updated = ss.Spec.Template.Spec.Containers[idx].Image != newImage
	if !updated {
		return
	}
	ss.Spec.Template.Spec.Containers[idx].Image = newImage
	err = c.client.Update(ctx, ss)
	return
}

func (c *K8sClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.client.Patch(ctx, obj, patch, opts...)
}

func (c *K8sClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return c.client.Create(ctx, obj, opts...)
}

func (c *K8sClient) CreateResourceIfNotExists(ctx context.Context, owner *aisv1.AIStore, res client.Object) (exists bool, err error) {
	if owner != nil {
		if err = controllerutil.SetControllerReference(owner, res, c.scheme); err != nil {
			return
		}
		res.SetNamespace(owner.Namespace)
	}

	err = c.client.Create(ctx, res)
	exists = err != nil && apierrors.IsAlreadyExists(err)
	if exists {
		err = nil
	}
	return
}

func (c *K8sClient) CreateOrUpdateResource(ctx context.Context, owner *aisv1.AIStore, res client.Object) (changed bool, err error) {
	exists, err := c.CreateResourceIfNotExists(ctx, owner, res)
	if err != nil {
		return false, err
	}

	if !exists {
		// resource create for first time
		return true, nil
	}

	key := client.ObjectKeyFromObject(res)
	existingObj := res.DeepCopyObject().(client.Object)
	if err := c.client.Get(ctx, key, existingObj); err != nil {
		return false, err
	}
	if equality.Semantic.DeepDerivative(res, existingObj) {
		return false, nil
	}
	err = c.client.Update(ctx, res)
	return err == nil, err
}

func (c *K8sClient) CheckIfNamespaceExists(ctx context.Context, name string) (exists bool, err error) {
	ns := &corev1.Namespace{}
	err = c.client.Get(ctx, types.NamespacedName{Name: name}, ns)
	if err == nil {
		exists = true
	} else if apierrors.IsNotFound(err) {
		err = nil
	}
	return exists, err
}

/////////////////////////////////
//       Delete resources      //
////////////////////////////////

// DeleteResourceIfExists deletes an existing resource. It doesn't fail if the resource does not exist
func (c *K8sClient) DeleteResourceIfExists(ctx context.Context, obj client.Object) (existed bool, err error) {
	err = c.client.Delete(ctx, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		err = fmt.Errorf("failed to delete %s: %q (namespace %q); err %v", obj.GetObjectKind(), obj.GetName(), obj.GetNamespace(), err)
		return false, err
	}
	return true, nil
}

func (c *K8sClient) DeleteServiceIfExists(ctx context.Context, name types.NamespacedName) (existed bool, err error) {
	svc := &corev1.Service{}
	svc.SetName(name.Name)
	svc.SetNamespace(name.Namespace)
	return c.DeleteResourceIfExists(ctx, svc)
}

func (c *K8sClient) DeleteAllServicesIfExist(ctx context.Context, namespace string, labels client.MatchingLabels) (anyExisted bool, err error) {
	svcs := &corev1.ServiceList{}
	err = c.client.List(ctx, svcs, client.InNamespace(namespace), labels)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		}
		return
	}

	for i := range svcs.Items {
		var existed bool
		existed, err = c.DeleteResourceIfExists(ctx, &svcs.Items[i])
		if err != nil {
			return
		}
		anyExisted = anyExisted || existed
	}
	return
}

func (c *K8sClient) DeletePVCs(ctx context.Context, namespace string, labels client.MatchingLabels, sc *string) (anyExisted bool, err error) {
	listOpts := []client.ListOption{client.InNamespace(namespace)}
	if labels != nil {
		listOpts = append(listOpts, labels)
	}
	pvcs := &corev1.PersistentVolumeClaimList{}
	err = c.client.List(ctx, pvcs, listOpts...)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		}
		return
	}
	return c.deleteAllPVCsIfExist(ctx, pvcs, sc)
}

func (c *K8sClient) deleteAllPVCsIfExist(ctx context.Context, pvcs *corev1.PersistentVolumeClaimList, sc *string) (anyExisted bool, err error) {
	for i := range pvcs.Items {
		if sc != nil && pvcs.Items[i].Spec.StorageClassName != nil && *pvcs.Items[i].Spec.StorageClassName != *sc {
			continue
		}
		var existed bool
		existed, err = c.DeleteResourceIfExists(ctx, &pvcs.Items[i])
		if err != nil {
			return
		}
		anyExisted = anyExisted || existed
	}
	return
}

func (c *K8sClient) DeleteStatefulSetIfExists(ctx context.Context, name types.NamespacedName) (existed bool, err error) {
	ss := &apiv1.StatefulSet{}
	ss.SetName(name.Name)
	ss.SetNamespace(name.Namespace)
	return c.DeleteResourceIfExists(ctx, ss)
}

func (c *K8sClient) DeleteConfigMapIfExists(ctx context.Context, name types.NamespacedName) (existed bool, err error) {
	ss := &corev1.ConfigMap{}
	ss.SetName(name.Name)
	ss.SetNamespace(name.Namespace)
	return c.DeleteResourceIfExists(ctx, ss)
}

func (c *K8sClient) DeletePodIfExists(ctx context.Context, name types.NamespacedName) (err error) {
	pod := &corev1.Pod{}
	pod.SetName(name.Name)
	pod.SetNamespace(name.Namespace)
	_, err = c.DeleteResourceIfExists(ctx, pod)
	return
}

func (c *K8sClient) WaitForPodReady(ctx context.Context, name types.NamespacedName, timeout time.Duration) error {
	var (
		retryInterval   = 3 * time.Second
		ctxBack, cancel = context.WithTimeout(ctx, timeout)
		pod             *corev1.Pod
		err             error
	)
	defer cancel()
	for {
		pod, err = c.GetPodByName(ctx, name)
		if err != nil {
			continue
		}
		if pod.Status.Phase == corev1.PodRunning {
			return nil
		}
		time.Sleep(retryInterval)
		select {
		case <-ctxBack.Done():
			return ctxBack.Err()
		default:
			break
		}
	}
}
