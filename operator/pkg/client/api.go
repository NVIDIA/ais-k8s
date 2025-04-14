// Package client contains wrapper for k8s client
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package client

import (
	"context"
	"fmt"
	"reflect"

	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/target"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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

func (c *K8sClient) ListPods(ctx context.Context, ais *aisv1.AIStore, labels map[string]string) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	err := c.client.List(ctx, podList, client.InNamespace(ais.Namespace), client.MatchingLabels(labels))
	return podList, err
}

func (c *K8sClient) ListJobsInNamespace(ctx context.Context, namespace string) (*batchv1.JobList, error) {
	jobList := &batchv1.JobList{}
	err := c.client.List(ctx, jobList, client.InNamespace(namespace))
	return jobList, err
}

func (c *K8sClient) GetStatefulSet(ctx context.Context, name types.NamespacedName) (*appsv1.StatefulSet, error) {
	return getResource[*appsv1.StatefulSet](c.client, ctx, name)
}

func (c *K8sClient) GetService(ctx context.Context, name types.NamespacedName) (*corev1.Service, error) {
	return getResource[*corev1.Service](c.client, ctx, name)
}

func (c *K8sClient) GetServiceEndpoints(ctx context.Context, svcName types.NamespacedName) (*corev1.Endpoints, error) {
	return getResource[*corev1.Endpoints](c.client, ctx, svcName)
}

func (c *K8sClient) GetConfigMap(ctx context.Context, name types.NamespacedName) (*corev1.ConfigMap, error) {
	return getResource[*corev1.ConfigMap](c.client, ctx, name)
}

func (c *K8sClient) GetPod(ctx context.Context, name types.NamespacedName) (*corev1.Pod, error) {
	return getResource[*corev1.Pod](c.client, ctx, name)
}

func (c *K8sClient) GetRole(ctx context.Context, name types.NamespacedName) (*rbacv1.Role, error) {
	return getResource[*rbacv1.Role](c.client, ctx, name)
}

func (c *K8sClient) Status() client.StatusWriter { return c.client.Status() }

// listPodsAndUpdateNodeNames lists pods based on the provided label selector and updates the uniqueNodeNames map with the node names of those pods
func (c *K8sClient) listPodsAndUpdateNodeNames(ctx context.Context, ais *aisv1.AIStore, labelSelector map[string]string, uniqueNodeNames sets.Set[string]) error {
	pods, err := c.ListPods(ctx, ais, labelSelector)
	if err != nil {
		return err
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		// check if the pod is running on a node (not failed or pending)
		if pod.Spec.NodeName != "" {
			uniqueNodeNames.Insert(pod.Spec.NodeName)
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
func (c *K8sClient) ListNodesRunningAIS(ctx context.Context, ais *aisv1.AIStore) ([]string, error) {
	uniqueNodeNames := sets.New[string]()
	if err := c.listPodsAndUpdateNodeNames(ctx, ais, proxy.RequiredPodLabels(ais), uniqueNodeNames); err != nil {
		return nil, err
	}
	if err := c.listPodsAndUpdateNodeNames(ctx, ais, target.RequiredPodLabels(ais), uniqueNodeNames); err != nil {
		return nil, err
	}
	return uniqueNodeNames.UnsortedList(), nil
}

//////////////////////////////////////
//      Create/Update resources     //
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
	logger := logf.FromContext(ctx).WithValues("statefulset", name.String())
	ss, err := c.GetStatefulSet(ctx, name)
	if err != nil {
		return
	}
	updated = *ss.Spec.Replicas != size
	if !updated {
		return
	}
	logger = logger.WithValues("before", *ss.Spec.Replicas, "after", size)
	logger.Info("Scaling statefulset")
	patch := client.MergeFrom(ss.DeepCopy())
	ss.Spec.Replicas = &size
	err = c.client.Patch(ctx, ss, patch)
	if err != nil {
		logger.Error(err, "Failed to scale statefulset")
		return
	}
	logger.Info("StatefulSet size updated")
	return
}

func (c *K8sClient) IsStatefulSetSize(ctx context.Context, name types.NamespacedName, size int32) (finished bool, err error) {
	logger := logf.FromContext(ctx).WithValues("statefulset", name.String(), "desiredSize", size)
	ss, err := c.GetStatefulSet(ctx, name)
	if err != nil {
		return
	}
	if ss.Status.Replicas != size {
		logger.Info("Statefulset replica count does not match desired size")
		return
	}
	return true, nil
}

func (c *K8sClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.client.Patch(ctx, obj, patch, opts...)
}

func (c *K8sClient) PatchIfExists(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	err := c.client.Patch(ctx, obj, patch, opts...)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
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

func (c *K8sClient) CreateOrUpdateResource(ctx context.Context, owner *aisv1.AIStore, res client.Object) (err error) {
	exists, err := c.CreateResourceIfNotExists(ctx, owner, res)
	if err != nil {
		return err
	}

	if !exists {
		// resource create for first time
		return nil
	}

	key := client.ObjectKeyFromObject(res)
	existingObj := res.DeepCopyObject().(client.Object)
	if err := c.client.Get(ctx, key, existingObj); err != nil {
		return err
	}
	if equality.Semantic.DeepDerivative(res, existingObj) {
		return nil
	}
	return c.client.Update(ctx, res)
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
/////////////////////////////////

// DeleteResourceIfExists deletes an existing resource. It doesn't fail if the resource does not exist
func (c *K8sClient) DeleteResourceIfExists(ctx context.Context, obj client.Object) (existed bool, err error) {
	err = c.client.Delete(ctx, obj)
	return allowObjNotFound(obj, err)
}

// DeleteResIfExistsWithGracePeriod deletes an existing resource with a specific grace period. It doesn't fail if the resource does not exist
func (c *K8sClient) DeleteResIfExistsWithGracePeriod(ctx context.Context, obj client.Object, gracePeriod int64) (existed bool, err error) {
	err = c.client.Delete(ctx, obj, client.GracePeriodSeconds(gracePeriod))
	return allowObjNotFound(obj, err)
}

func allowObjNotFound(obj client.Object, err error) (bool, error) {
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
	return deleteResourceIfExists[*corev1.Service](c, ctx, name)
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
	return deleteResourceIfExists[*appsv1.StatefulSet](c, ctx, name)
}

func (c *K8sClient) DeleteConfigMapIfExists(ctx context.Context, name types.NamespacedName) (existed bool, err error) {
	return deleteResourceIfExists[*corev1.ConfigMap](c, ctx, name)
}

func (c *K8sClient) DeletePodIfExists(ctx context.Context, name types.NamespacedName) (existed bool, err error) {
	return deleteResourceIfExists[*corev1.Pod](c, ctx, name)
}

func (c *K8sClient) GetReadyPod(ctx context.Context, name types.NamespacedName) (pod *corev1.Pod, err error) {
	pod, err = c.GetPod(ctx, name)
	if err != nil {
		return
	}
	if pod.Status.Phase != corev1.PodRunning {
		return pod, fmt.Errorf("pod is not yet running (phase: %s)", pod.Status.Phase)
	}
	return pod, nil
}

// GENERICS

func getResource[T client.Object](c client.Client, ctx context.Context, name types.NamespacedName) (T, error) { //nolint:revive // This is special case where it is just better to pass client first instead of context.
	var r T
	rv := reflect.New(reflect.TypeOf(r).Elem()).Interface().(T)
	err := c.Get(ctx, name, rv)
	return rv, err
}

func deleteResourceIfExists[T client.Object](c *K8sClient, ctx context.Context, name types.NamespacedName) (existed bool, err error) { //nolint:revive // This is special case where it is just better to pass client first instead of context.
	var r T
	rv := reflect.New(reflect.TypeOf(r).Elem()).Interface().(T)
	rv.SetName(name.Name)
	rv.SetNamespace(name.Namespace)
	return c.DeleteResourceIfExists(ctx, rv)
}
