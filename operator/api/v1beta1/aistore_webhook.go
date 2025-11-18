// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package v1beta1

import (
	"context"
	"fmt"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	"github.com/go-test/deep"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var webhooklog = logf.Log.WithName("aistore-resource")

// +kubebuilder:object:generate=false
type AIStoreWebhook struct {
	Client client.Client
}

// change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-ais-nvidia-com-v1beta1-aistore,mutating=false,failurePolicy=fail,sideEffects=None,groups=ais.nvidia.com,resources=aistores,verbs=create;update,versions=v1beta1,name=vaistore.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomValidator = &AIStoreWebhook{}

func (ais *AIStore) validateSize() (admission.Warnings, error) {
	if ais.Spec.ProxySpec.Size != nil && *ais.Spec.ProxySpec.Size <= 0 && !ais.IsProxyAutoScaling() {
		return nil, errInvalidDaemonSize(*ais.Spec.ProxySpec.Size, aisapc.Proxy)
	}

	if ais.Spec.TargetSpec.Size != nil && *ais.Spec.TargetSpec.Size <= 0 && !ais.IsTargetAutoScaling() {
		return nil, errInvalidDaemonSize(*ais.Spec.TargetSpec.Size, aisapc.Target)
	}

	// Validate `.spec.size` only when `.spec.targetSpec.size` or `.spec.proxySpec.size` is not set.
	if (ais.Spec.TargetSpec.Size == nil || ais.Spec.ProxySpec.Size == nil) && (ais.Spec.Size == nil || *ais.Spec.Size <= -2 || *ais.Spec.Size == 0) {
		return nil, errInvalidClusterSize(ais.Spec.Size)
	}

	return nil, nil
}

func (ais *AIStore) validateStateStorage() (admission.Warnings, error) {
	//nolint:all
	if ais.Spec.StateStorageClass != nil && ais.Spec.HostpathPrefix != nil {
		warning := fmt.Sprintf("Spec defines both hostpathPrefix and stateStorageClass. Using stateStorageClass %s", *ais.Spec.StateStorageClass)
		return []string{warning}, nil
	}
	if ais.Spec.StateStorageClass == nil && ais.Spec.HostpathPrefix == nil {
		return nil, errUndefinedStateStorage()
	}
	return nil, nil
}

func (ais *AIStore) validateAutoScaling() (admission.Warnings, error) {
	warns := admission.Warnings{}
	if ais.Spec.Size != nil && *ais.Spec.Size == -1 {
		if ais.Spec.TargetSpec.Size != nil && *ais.Spec.TargetSpec.Size != -1 {
			warns = append(warns, "spec.targetSpec.size is set when spec.Size is -1; defaulting to use the -1 of spec.Size")
		}
		if ais.Spec.ProxySpec.Size != nil && *ais.Spec.ProxySpec.Size != -1 {
			warns = append(warns, "spec.proxySpec.size is set when spec.Size is -1; defaulting to use the -1 of spec.Size")
		}
	}
	if ais.IsTargetAutoScaling() && ais.Spec.TargetSpec.NodeSelector == nil {
		return nil, errUndefinedNodeSelector("target")
	}
	if ais.IsProxyAutoScaling() && ais.Spec.ProxySpec.NodeSelector == nil {
		return nil, errUndefinedNodeSelector("proxy")
	}
	return warns, nil
}

func (ss *ServiceSpec) validate(path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for _, msg := range validation.IsValidPortNum(ss.ServicePort.IntValue()) {
		allErrs = append(allErrs, field.Invalid(path.Child("servicePort"), ss.ServicePort.IntValue(), msg))
	}
	for _, msg := range validation.IsValidPortNum(ss.PublicPort.IntValue()) {
		allErrs = append(allErrs, field.Invalid(path.Child("portPublic"), ss.PublicPort.IntValue(), msg))
	}
	for _, msg := range validation.IsValidPortNum(ss.IntraControlPort.IntValue()) {
		allErrs = append(allErrs, field.Invalid(path.Child("portIntraControl"), ss.IntraControlPort.IntValue(), msg))
	}
	for _, msg := range validation.IsValidPortNum(ss.IntraDataPort.IntValue()) {
		allErrs = append(allErrs, field.Invalid(path.Child("portIntraData"), ss.IntraDataPort.IntValue(), msg))
	}

	return allErrs
}

func (ais *AIStore) validateServiceSpec() (admission.Warnings, error) {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, ais.Spec.ProxySpec.validate(field.NewPath("spec", "proxySpec"))...)
	allErrs = append(allErrs, ais.Spec.TargetSpec.validate(field.NewPath("spec", "targetSpec"))...)

	return nil, allErrs.ToAggregate()
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (aisw *AIStoreWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ais, ok := obj.(*AIStore)
	if !ok {
		return nil, fmt.Errorf("failed to convert runtime.Object to AIStore")
	}

	webhooklog.WithValues("name", ais.Name, "namespace", ais.Namespace).Info("Validate create")
	return aisw.validateCreate(ctx, ais)
}

func (aisw *AIStoreWebhook) validateCreate(ctx context.Context, ais *AIStore) (admission.Warnings, error) {
	return ais.ValidateSpec(ctx,
		func() (admission.Warnings, error) {
			return aisw.verifyNodesAvailable(ctx, ais, aisapc.Proxy)
		},
		func() (admission.Warnings, error) {
			return aisw.verifyNodesAvailable(ctx, ais, aisapc.Target)
		},
		func() (admission.Warnings, error) {
			return aisw.verifyRequiredStorageClasses(ctx, ais)
		},
	)
}

func (ais *AIStore) ValidateSpec(_ context.Context, extraValidations ...func() (admission.Warnings, error)) (admission.Warnings, error) {
	var allWarnings admission.Warnings

	validations := []func() (admission.Warnings, error){
		ais.validateSize,
		ais.validateStateStorage,
		ais.validateAutoScaling,
		ais.validateServiceSpec,
	}
	validations = append(validations, extraValidations...)

	// Run each validation function, aggregate warnings, exit on error
	for _, validate := range validations {
		warnings, err := validate()
		if err != nil {
			return allWarnings, err
		}
		allWarnings = append(allWarnings, warnings...)
	}
	return allWarnings, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (aisw *AIStoreWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	prev, ok := oldObj.(*AIStore)
	if !ok {
		return nil, fmt.Errorf("failed to convert runtime.Object to AIStore")
	}
	ais, ok := newObj.(*AIStore)
	if !ok {
		return nil, fmt.Errorf("failed to convert runtime.Object to AIStore")
	}

	webhooklog.WithValues("name", ais.Name, "namespace", ais.Namespace).Info("Validate update")
	return aisw.validateUpdate(ctx, prev, ais)
}

func (aisw *AIStoreWebhook) validateUpdate(ctx context.Context, prev, ais *AIStore) (warnings admission.Warnings, err error) {
	if warnings, err = aisw.validateCreate(ctx, ais); err != nil {
		return warnings, err
	}

	// TODO: better validation, maybe using AIS IterFields?
	// users can update size for scaling up or down
	prev.Spec.ProxySpec.Size = ais.Spec.ProxySpec.Size
	prev.Spec.ProxySpec.Annotations = ais.Spec.ProxySpec.Annotations
	prev.Spec.ProxySpec.Labels = ais.Spec.ProxySpec.Labels
	prev.Spec.ProxySpec.Env = ais.Spec.ProxySpec.Env
	prev.Spec.ProxySpec.Resources = ais.Spec.ProxySpec.Resources
	prev.Spec.ProxySpec.SecurityContext = ais.Spec.ProxySpec.SecurityContext
	prev.Spec.ProxySpec.AutoScaleConf = ais.Spec.ProxySpec.AutoScaleConf
	if !equality.Semantic.DeepEqual(ais.Spec.ProxySpec, prev.Spec.ProxySpec) {
		diff := deep.Equal(ais.Spec.ProxySpec, prev.Spec.ProxySpec)
		webhooklog.Info(fmt.Sprintf("Differences found in proxy spec: [%s]", strings.Join(diff, ", ")))
		// TODO: For now, just error if proxy specs are updated. Eventually, this should be implemented.
		return warnings, errCannotUpdateSpec("proxySpec", diff...)
	}

	// same
	prev.Spec.TargetSpec.Size = ais.Spec.TargetSpec.Size
	prev.Spec.TargetSpec.Annotations = ais.Spec.TargetSpec.Annotations
	prev.Spec.TargetSpec.Labels = ais.Spec.TargetSpec.Labels
	prev.Spec.TargetSpec.Env = ais.Spec.TargetSpec.Env
	prev.Spec.TargetSpec.Resources = ais.Spec.TargetSpec.Resources
	prev.Spec.TargetSpec.SecurityContext = ais.Spec.TargetSpec.SecurityContext
	prev.Spec.TargetSpec.AutoScaleConf = ais.Spec.TargetSpec.AutoScaleConf
	if !equality.Semantic.DeepEqual(ais.Spec.TargetSpec, prev.Spec.TargetSpec) {
		diff := deep.Equal(ais.Spec.TargetSpec, prev.Spec.TargetSpec)
		webhooklog.Info(fmt.Sprintf("Differences found in target spec: [%s]", strings.Join(diff, ", ")))
		// TODO: For now, just error if target specs are updated. Eventually, this should be implemented.
		return warnings, errCannotUpdateSpec("targetSpec", diff...)
	}

	if ais.Spec.EnableExternalLB != prev.Spec.EnableExternalLB {
		return warnings, errCannotUpdateSpec("enableExternalLB")
	}

	if ais.Spec.HostpathPrefix != nil && prev.Spec.HostpathPrefix != nil {
		if *ais.Spec.HostpathPrefix != *prev.Spec.HostpathPrefix {
			return warnings, errCannotUpdateSpec("hostpathPrefix")
		}
	}

	return
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (*AIStoreWebhook) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ais, ok := obj.(*AIStore)
	if !ok {
		return nil, fmt.Errorf("failed to convert runtime.Object to AIStore")
	}

	webhooklog.WithValues("name", ais.Name, "namespace", ais.Namespace).Info("Validate delete")
	return nil, nil
}

func (aisw *AIStoreWebhook) verifyNodesAvailable(ctx context.Context, ais *AIStore, daeType string) (admission.Warnings, error) {
	var (
		requiredSize int
		nodeSelector map[string]string
		nodes        = &corev1.NodeList{}
	)
	switch daeType {
	case aisapc.Proxy:
		requiredSize = int(ais.GetProxySize())
		nodeSelector = ais.Spec.ProxySpec.NodeSelector
	case aisapc.Target:
		if ais.AllowTargetSharedNodes() {
			return nil, nil
		}
		requiredSize = int(ais.GetTargetSize())
		nodeSelector = ais.Spec.TargetSpec.NodeSelector
	default:
		return nil, fmt.Errorf("invalid daemon type: %s", daeType)
	}

	// Check that desired nodes matching this selector does not exceed available K8s cluster nodes
	err := aisw.Client.List(ctx, nodes, &client.ListOptions{LabelSelector: labels.SelectorFromSet(nodeSelector)})
	if err != nil {
		return nil, err
	}
	if len(nodes.Items) >= requiredSize {
		return nil, nil
	}
	return admission.Warnings{
		fmt.Sprintf("spec for AIS %s requires more K8s nodes matching the given selector: expected '%d' but found '%d'", daeType, requiredSize, len(nodes.Items)),
	}, nil
}

// Ensure all storage classes requested by the AIS resource are available in the cluster
func (aisw *AIStoreWebhook) verifyRequiredStorageClasses(ctx context.Context, ais *AIStore) (admission.Warnings, error) {
	scList := &storagev1.StorageClassList{}
	err := aisw.Client.List(ctx, scList)
	if err != nil {
		return nil, err
	}
	scMap := make(map[string]*storagev1.StorageClass, len(scList.Items))
	for i := range scList.Items {
		scMap[scList.Items[i].Name] = &scList.Items[i]
	}

	requiredClasses := []*string{ais.Spec.StateStorageClass}
	for _, requiredClass := range requiredClasses {
		if requiredClass != nil {
			if _, exists := scMap[*requiredClass]; !exists {
				return nil, fmt.Errorf("required storage class '%s' not found", *requiredClass)
			}
		}
	}
	return nil, nil
}

// errors
func errInvalidClusterSize(size *int32) error {
	if size == nil {
		return fmt.Errorf("cluster size is not specified")
	}
	return fmt.Errorf("invalid cluster size %d, should be at least 1 or -1 for autoScaling", *size)
}

// errors
func errInvalidDaemonSize(size int32, daeType string) error {
	return fmt.Errorf("invalid %s daemon size %d, should be at least 1", daeType, size)
}

func errCannotUpdateSpec(specName string, diff ...string) error {
	if len(diff) > 0 {
		return fmt.Errorf("cannot update spec %q for an existing cluster, diff: [%s]", specName, strings.Join(diff, ", "))
	}
	return fmt.Errorf("cannot update spec %q for an existing cluster", specName)
}

func errUndefinedStateStorage() error {
	return fmt.Errorf("AIS spec does not define hostpathPrefix or stateStorageClass. Set hostpathPrefix to use a directory on each node or set stateStorageClass to use a dynamic storage class")
}

func errUndefinedNodeSelector(spec string) error {
	return fmt.Errorf("missing nodeSelector for %s; nodeSelector is required when autoScale is enabled", spec)
}
