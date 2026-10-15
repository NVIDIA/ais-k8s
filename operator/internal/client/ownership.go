/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/ais-operator/internal/resources/ownerref"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// applyOwned performs a Server-Side Apply under the operator's field manager.
// ForceOwnership lets reconciles overwrite external changes (e.g. a kubectl
// edit) to fields we declare. applyOwned fails for an existing object whose
// controller is not the controller in objMeta.
func (c *K8sClient) applyOwned(ctx context.Context, existing client.Object, ac runtime.ApplyConfiguration, objMeta *metav1ac.ObjectMetaApplyConfiguration) error {
	key := types.NamespacedName{Name: *objMeta.Name}
	if objMeta.Namespace != nil {
		key.Namespace = *objMeta.Namespace
	}
	rv, err := c.ownedResourceVersion(ctx, key, existing, declaredController(objMeta.OwnerReferences))
	if err != nil {
		return err
	}
	return c.applyAtVersion(ctx, ac, objMeta, rv)
}

// applyAtVersion applies ac with an optional resourceVersion precondition.
func (c *K8sClient) applyAtVersion(ctx context.Context, ac runtime.ApplyConfiguration, objMeta *metav1ac.ObjectMetaApplyConfiguration, rv string) error {
	if rv != "" {
		objMeta.WithResourceVersion(rv)
	}
	return c.client.Apply(ctx, ac, client.FieldOwner(FieldOwner), client.ForceOwnership)
}

// deleteOwned deletes the object at key if owner owns it. It leaves a missing or non-owned object unchanged.
func (c *K8sClient) deleteOwned(ctx context.Context, key types.NamespacedName, existing, owner client.Object) error {
	rv, err := c.ownedResourceVersion(ctx, key, existing, owner)
	if errors.Is(err, errNotOwned) {
		logf.FromContext(ctx).V(1).Info("Leaving non-owned object unchanged", "key", key)
		return nil
	}
	if err != nil || rv == "" {
		return err
	}
	uid := existing.GetUID()
	_, err = c.DeleteResourceIfExists(ctx, existing, client.Preconditions{UID: &uid, ResourceVersion: &rv})
	return err
}

var errNotOwned = errors.New("exists but is not owned by its declared owner")

// ownedResourceVersion returns the resourceVersion of the object at key, or "" if the object does not exist.
// It fails with errNotOwned for an existing object that owner does not own.
func (c *K8sClient) ownedResourceVersion(
	ctx context.Context, key types.NamespacedName, existing, owner client.Object,
) (string, error) {
	err := c.client.Get(ctx, key, existing)
	switch {
	case apierrors.IsNotFound(err):
		return "", nil
	case err != nil:
		return "", err
	case !c.isOwnedBy(existing, owner):
		ref := key.String()
		if key.Namespace == "" {
			ref = key.Name
		}
		return "", fmt.Errorf("%s %w", ref, errNotOwned)
	}
	return existing.GetResourceVersion(), nil
}

// isOwnedBy reports whether owner owns obj.
func (c *K8sClient) isOwnedBy(obj, owner client.Object) bool {
	if owner == nil {
		return false
	}
	// Kubernetes does not resolve an OwnerReference from a cluster-scoped object to a namespaced owner,
	// so labels record the owner.
	if obj.GetNamespace() == "" && owner.GetNamespace() != "" {
		recorded, ok := ownerref.GetLabeledOwner(obj)
		want := ownerref.LabeledOwner{Kind: c.kindOf(owner), NamespacedName: client.ObjectKeyFromObject(owner)}
		return ok && recorded == want
	}
	return metav1.IsControlledBy(obj, owner)
}

// kindOf returns the kind of obj, or "" if the kind is unknown.
func (c *K8sClient) kindOf(obj client.Object) string {
	if kind := obj.GetObjectKind().GroupVersionKind().Kind; kind != "" {
		return kind
	}
	gvk, err := apiutil.GVKForObject(obj, c.client.Scheme())
	if err != nil {
		return ""
	}
	return gvk.Kind
}

// declaredController returns the controller in refs, or nil if refs declare none.
func declaredController(refs []metav1ac.OwnerReferenceApplyConfiguration) client.Object {
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller && refs[i].UID != nil {
			return &metav1.PartialObjectMetadata{UID: *refs[i].UID}
		}
	}
	return nil
}
