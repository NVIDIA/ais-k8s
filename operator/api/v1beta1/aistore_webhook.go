// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
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
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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

var _ admission.Validator[*AIStore] = &AIStoreWebhook{}

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
	if ais.Spec.StateStorage != nil {
		if ais.Spec.StateStorageClass != nil || ais.Spec.HostpathPrefix != nil {
			warnings := admission.Warnings{"spec.stateStorage is set; ignoring legacy hostpathPrefix and stateStorageClass fields"}
			if !ais.Spec.hasExactlyOneStateStorageMode() {
				return warnings, errInvalidStateStorage()
			}
			return warnings, nil
		}
		if !ais.Spec.hasExactlyOneStateStorageMode() {
			return nil, errInvalidStateStorage()
		}
		return nil, nil
	}
	if ais.Spec.StateStorageClass != nil && ais.Spec.HostpathPrefix != nil {
		warning := fmt.Sprintf("Spec defines both hostpathPrefix and stateStorageClass. Using stateStorageClass %s", *ais.Spec.StateStorageClass)
		return admission.Warnings{warning}, nil
	}
	if ais.Spec.StateStorageClass == nil && ais.Spec.HostpathPrefix == nil {
		return nil, errUndefinedStateStorage()
	}
	return nil, nil
}

func (s *AIStoreSpec) hasExactlyOneStateStorageMode() bool {
	count := 0
	if s.StateStorage == nil {
		return false
	}
	if s.StateStorage.HostPath != nil {
		count++
	}
	if s.StateStorage.PVC != nil {
		count++
	}
	return count == 1
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
	svcMsgs := validation.IsValidPortNum(ss.ServicePort.IntValue())
	pubMsgs := validation.IsValidPortNum(ss.PublicPort.IntValue())
	ctrlMsgs := validation.IsValidPortNum(ss.IntraControlPort.IntValue())
	dataMsgs := validation.IsValidPortNum(ss.IntraDataPort.IntValue())

	allErrs := make(field.ErrorList, 0, len(svcMsgs)+len(pubMsgs)+len(ctrlMsgs)+len(dataMsgs))
	for _, msg := range svcMsgs {
		allErrs = append(allErrs, field.Invalid(path.Child("servicePort"), ss.ServicePort.IntValue(), msg))
	}
	for _, msg := range pubMsgs {
		allErrs = append(allErrs, field.Invalid(path.Child("portPublic"), ss.PublicPort.IntValue(), msg))
	}
	for _, msg := range ctrlMsgs {
		allErrs = append(allErrs, field.Invalid(path.Child("portIntraControl"), ss.IntraControlPort.IntValue(), msg))
	}
	for _, msg := range dataMsgs {
		allErrs = append(allErrs, field.Invalid(path.Child("portIntraData"), ss.IntraDataPort.IntValue(), msg))
	}

	return allErrs
}

func (ais *AIStore) validateServiceSpec() (admission.Warnings, error) {
	proxyErrs := ais.Spec.ProxySpec.validate(field.NewPath("spec", "proxySpec"))
	targetErrs := ais.Spec.TargetSpec.validate(field.NewPath("spec", "targetSpec"))

	allErrs := make(field.ErrorList, 0, len(proxyErrs)+len(targetErrs))
	allErrs = append(allErrs, proxyErrs...)
	allErrs = append(allErrs, targetErrs...)

	return nil, allErrs.ToAggregate()
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (aisw *AIStoreWebhook) ValidateCreate(ctx context.Context, ais *AIStore) (admission.Warnings, error) {
	webhooklog.WithValues("name", ais.Name, "namespace", ais.Namespace).Info("Validate create")
	return aisw.validateSpec(ctx, ais)
}

func (aisw *AIStoreWebhook) validateSpec(ctx context.Context, ais *AIStore) (admission.Warnings, error) {
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

// validateTLSCertPaths rejects specs that set both spec.tls and any of the cert path
// fields (server_crt, server_key, client_ca_tls) in configToUpdate.net.http, since
// the operator manages those paths under /var/certs and would silently override them.
func (ais *AIStore) validateTLSCertPaths() (admission.Warnings, error) {
	if ais.Spec.TLS == nil || ais.Spec.ConfigToUpdate == nil || ais.Spec.ConfigToUpdate.Net == nil || ais.Spec.ConfigToUpdate.Net.HTTP == nil {
		return nil, nil
	}
	http := ais.Spec.ConfigToUpdate.Net.HTTP
	var conflicts []string
	if http.Certificate != nil {
		conflicts = append(conflicts, "server_crt")
	}
	if http.CertKey != nil {
		conflicts = append(conflicts, "server_key")
	}
	if http.ClientCA != nil {
		conflicts = append(conflicts, "client_ca_tls")
	}
	if len(conflicts) == 0 {
		return nil, nil
	}
	return nil, fmt.Errorf("configToUpdate.net.http.[%s] cannot be set together with spec.tls; the operator manages cert paths under /var/certs", strings.Join(conflicts, ","))
}

func (ais *AIStore) validateCleanupConfig() (admission.Warnings, error) {
	if !ais.ShouldCleanupMetadata() {
		return nil, nil
	}
	if !ais.Spec.UsesStateHostPath() {
		return nil, nil
	}
	if len(ais.Spec.TargetSpec.NodeSelector) == 0 || len(ais.Spec.ProxySpec.NodeSelector) == 0 {
		return admission.Warnings{
			"cleanupMetadata is enabled with hostpath state and empty nodeSelector; host cleanup jobs will run on ALL nodes in the cluster",
		}, nil
	}
	return nil, nil
}

func (ais *AIStore) ValidateSpec(_ context.Context, extraValidations ...func() (admission.Warnings, error)) (admission.Warnings, error) {
	var allWarnings admission.Warnings
	base := []func() (admission.Warnings, error){
		ais.validateSize,
		ais.validateStateStorage,
		ais.validateAutoScaling,
		ais.validateServiceSpec,
		ais.validateCleanupConfig,
		ais.validateTLSCertPaths,
	}

	validations := make(
		[]func() (admission.Warnings, error),
		0,
		len(base)+len(extraValidations),
	)
	validations = append(validations, base...)
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
func (aisw *AIStoreWebhook) ValidateUpdate(ctx context.Context, prev, ais *AIStore) (admission.Warnings, error) {
	webhooklog.WithValues("name", ais.Name, "namespace", ais.Namespace).Info("Validate update")
	warnings, err := aisw.validateSpec(ctx, ais)
	if err != nil {
		return warnings, err
	}

	// TODO: better validation, maybe using AIS IterFields?
	err = validateProxyUpdate(prev, ais)
	if err != nil {
		return warnings, err
	}
	// same
	err = validateTargetUpdate(prev, ais)
	if err != nil {
		return warnings, err
	}

	if ais.Spec.EnableExternalLB != prev.Spec.EnableExternalLB {
		return warnings, errCannotUpdateSpec("enableExternalLB")
	}
	if storageErr := validateStateStorageUpdate(prev, ais); storageErr != nil {
		return warnings, storageErr
	}
	return warnings, nil
}

// allowDaemonSpecUpdates copies fields from `ais` onto `prev` that are allowed
// to change on an existing cluster. Any field not copied here will cause the
// update to be rejected if it differs from the previous value.
func allowDaemonSpecUpdates(prev, spec *DaemonSpec) {
	prev.Size = spec.Size
	prev.Annotations = spec.Annotations
	prev.Labels = spec.Labels
	prev.Env = spec.Env
	prev.Resources = spec.Resources
	prev.SecurityContext = spec.SecurityContext
	prev.Capabilities = spec.Capabilities
	prev.AISContainerSecurityContext = spec.AISContainerSecurityContext
	prev.AutoScaleConf = spec.AutoScaleConf
	prev.PVCRetentionPolicy = spec.PVCRetentionPolicy
	prev.Probes = spec.Probes
	prev.Tolerations = spec.Tolerations
}

func validateProxyUpdate(prev, ais *AIStore) error {
	allowDaemonSpecUpdates(&prev.Spec.ProxySpec, &ais.Spec.ProxySpec)
	if !equality.Semantic.DeepEqual(ais.Spec.ProxySpec, prev.Spec.ProxySpec) {
		diff := deep.Equal(ais.Spec.ProxySpec, prev.Spec.ProxySpec)
		webhooklog.Info(fmt.Sprintf("Differences found in proxy spec: [%s]", strings.Join(diff, ", ")))
		return errCannotUpdateSpec("proxySpec", diff...)
	}
	return nil
}

func validateTargetUpdate(prev, ais *AIStore) error {
	allowDaemonSpecUpdates(&prev.Spec.TargetSpec.DaemonSpec, &ais.Spec.TargetSpec.DaemonSpec)
	prev.Spec.TargetSpec.PodDisruptionBudget = ais.Spec.TargetSpec.PodDisruptionBudget
	if !equality.Semantic.DeepEqual(ais.Spec.TargetSpec, prev.Spec.TargetSpec) {
		diff := deep.Equal(ais.Spec.TargetSpec, prev.Spec.TargetSpec)
		webhooklog.Info(fmt.Sprintf("Differences found in target spec: [%s]", strings.Join(diff, ", ")))
		return errCannotUpdateSpec("targetSpec", diff...)
	}
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (*AIStoreWebhook) ValidateDelete(_ context.Context, ais *AIStore) (admission.Warnings, error) {
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

	requiredClasses := []*string{ais.Spec.StateStoragePVCStorageClass()}
	for _, requiredClass := range requiredClasses {
		if requiredClass != nil {
			if _, exists := scMap[*requiredClass]; !exists {
				return nil, fmt.Errorf("required storage class '%s' not found", *requiredClass)
			}
		}
	}
	return nil, nil
}

func validateStateStorageUpdate(prev, ais *AIStore) error {
	// Allow updates from legacy fields without modification
	if !equality.Semantic.DeepEqual(ais.Spec.StateStorageHostPathPrefix(), prev.Spec.StateStorageHostPathPrefix()) {
		return errCannotUpdateSpec("stateStorage.hostPath.prefix")
	}
	if !equality.Semantic.DeepEqual(ais.Spec.StateStoragePVCStorageClass(), prev.Spec.StateStoragePVCStorageClass()) {
		return errCannotUpdateSpec("stateStorage.pvc.storageClass")
	}
	return nil
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
	return fmt.Errorf("AIS spec does not define stateStorage. Set stateStorage.hostPath or stateStorage.pvc")
}

func errInvalidStateStorage() error {
	return fmt.Errorf("AIS spec stateStorage must define exactly one of hostPath or pvc")
}

func errUndefinedNodeSelector(spec string) error {
	return fmt.Errorf("missing nodeSelector for %s; nodeSelector is required when autoScale is enabled", spec)
}
