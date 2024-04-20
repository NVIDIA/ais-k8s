// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package v1beta1

import (
	"context"
	"fmt"
	"reflect"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var aistorelog = logf.Log.WithName("aistore-resource")

// +kubebuilder:object:generate=false
type AIStoreWebhook struct{}

// change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-ais-nvidia-com-v1beta1-aistore,mutating=false,failurePolicy=fail,sideEffects=None,groups=ais.nvidia.com,resources=aistores,verbs=create;update,versions=v1beta1,name=vaistore.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomValidator = &AIStoreWebhook{}

func (ais *AIStore) validateSize() (admission.Warnings, error) {
	if ais.Spec.ProxySpec.Size != nil && *ais.Spec.ProxySpec.Size <= 0 {
		return nil, errInvalidDaemonSize(*ais.Spec.ProxySpec.Size, aisapc.Proxy)
	}

	if ais.Spec.TargetSpec.Size != nil && *ais.Spec.TargetSpec.Size <= 0 {
		return nil, errInvalidDaemonSize(*ais.Spec.TargetSpec.Size, aisapc.Target)
	}

	// Validate `.spec.size` only when `.spec.targetSpec.size` or `.spec.proxySpec.size` is not set.
	if (ais.Spec.TargetSpec.Size == nil || ais.Spec.ProxySpec.Size == nil) && ais.Spec.Size <= 0 {
		return nil, errInvalidClusterSize(ais.Spec.Size)
	}

	return nil, nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (*AIStoreWebhook) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ais, ok := obj.(*AIStore)
	if !ok {
		return nil, fmt.Errorf("failed to covert runtime.Object to AIStore")
	}

	aistorelog.Info("Validate create", "name", ais.Name)
	return ais.validateSize()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (*AIStoreWebhook) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	prev, ok := oldObj.(*AIStore)
	if !ok {
		return nil, fmt.Errorf("failed to covert runtime.Object to AIStore")
	}
	ais, ok := newObj.(*AIStore)
	if !ok {
		return nil, fmt.Errorf("failed to covert runtime.Object to AIStore")
	}

	aistorelog.Info("Validate update", "name", ais.Name)

	return nil, ais.vup(prev)
}

func (ais *AIStore) vup(prev *AIStore) error {
	if _, err := ais.validateSize(); err != nil {
		return err
	}

	// TODO: better validation, maybe using AIS IterFields?
	// users can update size for scaling up or down
	prev.Spec.ProxySpec.Size = ais.Spec.ProxySpec.Size
	if !reflect.DeepEqual(ais.Spec.ProxySpec, prev.Spec.ProxySpec) {
		return errCannotUpdateSpec("proxySpec")
	}

	// same
	prev.Spec.TargetSpec.Size = ais.Spec.TargetSpec.Size
	if !reflect.DeepEqual(ais.Spec.TargetSpec, prev.Spec.TargetSpec) {
		// TODO: For now, just log error if target specs are updated. Eventually, implement
		// logic that compares target specs accurately.
		err := errCannotUpdateSpec("targetSpec")
		aistorelog.Error(err, fmt.Sprintf("%v != %v", ais.Spec.TargetSpec, prev.Spec.TargetSpec))
	}

	if !reflect.DeepEqual(ais.Spec.DisablePodAntiAffinity, prev.Spec.DisablePodAntiAffinity) {
		return errCannotUpdateSpec("disablePodAntiAffinity")
	}

	if ais.Spec.EnableExternalLB != prev.Spec.EnableExternalLB {
		return errCannotUpdateSpec("enableExternalLB")
	}

	if ais.Spec.HostpathPrefix != prev.Spec.HostpathPrefix {
		return errCannotUpdateSpec("hostpathPrefix")
	}
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (*AIStoreWebhook) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ais, ok := obj.(*AIStore)
	if !ok {
		return nil, fmt.Errorf("failed to covert runtime.Object to AIStore")
	}

	aistorelog.Info("Validate delete", "name", ais.Name)
	return nil, nil
}

// errors
func errInvalidClusterSize(size int32) error {
	return fmt.Errorf("invalid cluster size %d, should be at least 1", size)
}

// errors
func errInvalidDaemonSize(size int32, daeType string) error {
	return fmt.Errorf("invalid %s daemon size %d, should be at least 1", daeType, size)
}

func errCannotUpdateSpec(specName string) error {
	return fmt.Errorf("cannot update spec %q for an existing cluster", specName)
}
