/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package v1beta1 contains admission webhooks for the ais.nvidia.com/v1beta1 API group.
package v1beta1

import (
	"context"
	"fmt"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	webhookcmn "github.com/ais-operator/internal/webhook"
	"github.com/go-test/deep"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// webhooklog is for logging in this package.
var webhooklog = logf.Log.WithName("aistore-resource")

// +kubebuilder:object:generate=false

// AIStoreWebhook validates AIStore resources on admission.
type AIStoreWebhook struct {
	Client client.Client
}

// change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-ais-nvidia-com-v1beta1-aistore,mutating=false,failurePolicy=fail,sideEffects=None,groups=ais.nvidia.com,resources=aistores,verbs=create;update,versions=v1beta1,name=vaistore.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

var _ admission.Validator[*aisv1.AIStore] = &AIStoreWebhook{}

// ValidateCreate implements admission.Validator.
func (aisw *AIStoreWebhook) ValidateCreate(ctx context.Context, ais *aisv1.AIStore) (admission.Warnings, error) {
	webhooklog.WithValues("name", ais.Name, "namespace", ais.Namespace).Info("Validate create")
	return aisw.validateSpec(ctx, nil, ais)
}

// ValidateUpdate implements admission.Validator.
func (aisw *AIStoreWebhook) ValidateUpdate(ctx context.Context, prev, ais *aisv1.AIStore) (admission.Warnings, error) {
	webhooklog.WithValues("name", ais.Name, "namespace", ais.Namespace).Info("Validate update")
	warnings, err := aisw.validateSpec(ctx, prev, ais)
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

	if ais.Spec.EnableExternalLB != prev.Spec.EnableExternalLB { //nolint:staticcheck // deprecated EnableExternalLB field
		return warnings, errCannotUpdateSpec("enableExternalLB")
	}
	if storageErr := validateStateStorageUpdate(prev, ais); storageErr != nil {
		return warnings, storageErr
	}
	return warnings, nil
}

// ValidateDelete implements admission.Validator.
func (*AIStoreWebhook) ValidateDelete(_ context.Context, ais *aisv1.AIStore) (admission.Warnings, error) {
	webhooklog.WithValues("name", ais.Name, "namespace", ais.Namespace).Info("Validate delete")
	return nil, nil
}

// validateSpec runs the spec-only validations defined on the AIStore type, then
// runs webhook-only validations that require admission or cluster context.
func (aisw *AIStoreWebhook) validateSpec(ctx context.Context, prev, ais *aisv1.AIStore) (admission.Warnings, error) {
	allWarnings, err := ais.ValidateSpec(ctx)
	if err != nil {
		return allWarnings, err
	}

	warnings, err := aisw.verifyNodesAvailable(ctx, ais, aisapc.Proxy)
	if err != nil {
		return allWarnings, err
	}
	allWarnings = append(allWarnings, warnings...)

	warnings, err = aisw.verifyNodesAvailable(ctx, ais, aisapc.Target)
	if err != nil {
		return allWarnings, err
	}
	allWarnings = append(allWarnings, warnings...)

	err = aisw.verifyRequiredStorageClasses(ctx, ais)
	if err != nil {
		return allWarnings, err
	}

	err = aisw.validateAuthAccess(ctx, prev, ais)
	return allWarnings, err
}

func authSecretNamespace(ais *aisv1.AIStore, up *aisv1.UsernamePasswordAuth) string {
	if up.SecretNamespace != nil {
		return *up.SecretNamespace
	}
	return ais.Namespace
}

// validateAuthSecret checks user access to spec.auth.usernamePassword:
// requires "get" on the referenced credentials Secret, checked on every create and update when changed
func (aisw *AIStoreWebhook) validateAuthSecret(ctx context.Context, prev, ais *aisv1.AIStore) error {
	if ais.Spec.Auth == nil || ais.Spec.Auth.UsernamePassword == nil { //nolint:staticcheck // deprecated UsernamePassword field
		return nil
	}
	up := ais.Spec.Auth.UsernamePassword                                                                  //nolint:staticcheck // deprecated UsernamePassword field
	previousEntryExists := prev != nil && prev.Spec.Auth != nil && prev.Spec.Auth.UsernamePassword != nil //nolint:staticcheck // deprecated UsernamePassword field
	// Skip SubjectAccessReview if the reference is unchanged
	if previousEntryExists {
		previousUP := prev.Spec.Auth.UsernamePassword //nolint:staticcheck // deprecated UsernamePassword field
		if previousUP.SecretName == up.SecretName && authSecretNamespace(prev, previousUP) == authSecretNamespace(ais, up) {
			return nil
		}
	}
	return aisw.authorize(ctx, ais, "get", field.NewPath("spec", "auth", "usernamePassword"),
		&authorizationv1.ResourceAttributes{
			Resource:  "secrets",
			Namespace: authSecretNamespace(ais, up),
			Name:      up.SecretName,
		})
}

// validateAuthProfile checks user access to spec.auth.profileRef:
// requires "use" on the referenced AIStoreAuthProfile, checked on every create and update when changed
func (aisw *AIStoreWebhook) validateAuthProfile(ctx context.Context, prev, ais *aisv1.AIStore) error {
	if ais.Spec.Auth == nil || ais.Spec.Auth.ProfileRef == nil {
		return nil
	}
	ref := ais.Spec.Auth.ProfileRef
	previousEntryExists := prev != nil && prev.Spec.Auth != nil && prev.Spec.Auth.ProfileRef != nil
	// Skip SubjectAccessReview if the reference is unchanged
	if previousEntryExists && prev.Spec.Auth.ProfileRef.Name == ref.Name {
		return nil
	}
	path := field.NewPath("spec", "auth", "profileRef")
	err := aisw.authorize(ctx, ais, "use", path,
		&authorizationv1.ResourceAttributes{
			Group:    authv1alpha1.GroupVersion.Group,
			Version:  authv1alpha1.GroupVersion.Version,
			Resource: "aistoreauthprofiles",
			Name:     ref.Name,
		})
	if err != nil {
		return err
	}
	return aisw.validateAuthProfileExistence(ctx, path, ais.Name, ref.Name)
}

// validateAuthProfileExistence checks if a given AIStoreAuthProfile exists using operator permissions
func (aisw *AIStoreWebhook) validateAuthProfileExistence(ctx context.Context, path *field.Path, aisName, profName string) error {
	prof := &authv1alpha1.AIStoreAuthProfile{}
	if err := aisw.Client.Get(ctx, client.ObjectKey{Name: profName}, prof); err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewInvalid(
				aisv1.GroupVersion.WithKind("AIStore").GroupKind(),
				aisName,
				field.ErrorList{field.Invalid(path, profName, "referenced AIStoreAuthProfile does not exist")},
			)
		}
		return apierrors.NewInternalError(
			fmt.Errorf("checking AIStoreAuthProfile %q: %w", profName, err),
		)
	}
	return nil
}

// validateAuthAccess verifies the submitting user may use the referenced auth configuration.
func (aisw *AIStoreWebhook) validateAuthAccess(ctx context.Context, prev, ais *aisv1.AIStore) error {
	if err := aisw.validateAuthProfile(ctx, prev, ais); err != nil {
		return err
	}
	return aisw.validateAuthSecret(ctx, prev, ais)
}

func (aisw *AIStoreWebhook) authorize(
	ctx context.Context,
	ais *aisv1.AIStore,
	verb string,
	path *field.Path,
	attrs *authorizationv1.ResourceAttributes,
) error {
	fieldErr, err := webhookcmn.Authorize(ctx, aisw.Client, verb, path, attrs)
	if err != nil || fieldErr == nil {
		return err
	}
	return apierrors.NewInvalid(
		aisv1.GroupVersion.WithKind("AIStore").GroupKind(),
		ais.Name,
		field.ErrorList{fieldErr},
	)
}

// allowDaemonSpecUpdates copies fields from `ais` onto `prev` that are allowed
// to change on an existing cluster. Any field not copied here will cause the
// update to be rejected if it differs from the previous value.
func allowDaemonSpecUpdates(prev, spec *aisv1.DaemonSpec) {
	prev.Size = spec.Size
	prev.Annotations = spec.Annotations
	prev.Labels = spec.Labels
	prev.Env = spec.Env
	prev.Resources = spec.Resources
	prev.SecurityContext = spec.SecurityContext
	prev.Capabilities = spec.Capabilities //nolint:staticcheck // deprecated Capabilities field
	prev.AISContainerSecurityContext = spec.AISContainerSecurityContext
	prev.AutoScaleConf = spec.AutoScaleConf
	prev.PVCRetentionPolicy = spec.PVCRetentionPolicy
	prev.Probes = spec.Probes
	prev.Tolerations = spec.Tolerations
}

func validateProxyUpdate(prev, ais *aisv1.AIStore) error {
	allowDaemonSpecUpdates(&prev.Spec.ProxySpec, &ais.Spec.ProxySpec)
	if !equality.Semantic.DeepEqual(ais.Spec.ProxySpec, prev.Spec.ProxySpec) {
		diff := deep.Equal(ais.Spec.ProxySpec, prev.Spec.ProxySpec)
		webhooklog.Info(fmt.Sprintf("Differences found in proxy spec: [%s]", strings.Join(diff, ", ")))
		return errCannotUpdateSpec("proxySpec", diff...)
	}
	return nil
}

func validateTargetUpdate(prev, ais *aisv1.AIStore) error {
	allowDaemonSpecUpdates(&prev.Spec.TargetSpec.DaemonSpec, &ais.Spec.TargetSpec.DaemonSpec)
	prev.Spec.TargetSpec.PodDisruptionBudget = ais.Spec.TargetSpec.PodDisruptionBudget
	prev.Spec.TargetSpec.ScaleDownMode = ais.Spec.TargetSpec.ScaleDownMode
	if !equality.Semantic.DeepEqual(ais.Spec.TargetSpec, prev.Spec.TargetSpec) {
		diff := deep.Equal(ais.Spec.TargetSpec, prev.Spec.TargetSpec)
		webhooklog.Info(fmt.Sprintf("Differences found in target spec: [%s]", strings.Join(diff, ", ")))
		return errCannotUpdateSpec("targetSpec", diff...)
	}
	return nil
}

func (aisw *AIStoreWebhook) verifyNodesAvailable(ctx context.Context, ais *aisv1.AIStore, daeType string) (admission.Warnings, error) {
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
func (aisw *AIStoreWebhook) verifyRequiredStorageClasses(ctx context.Context, ais *aisv1.AIStore) error {
	scList := &storagev1.StorageClassList{}
	err := aisw.Client.List(ctx, scList)
	if err != nil {
		return err
	}
	scMap := make(map[string]*storagev1.StorageClass, len(scList.Items))
	for i := range scList.Items {
		scMap[scList.Items[i].Name] = &scList.Items[i]
	}

	requiredClasses := []*string{ais.Spec.StateStoragePVCStorageClass()}
	for _, requiredClass := range requiredClasses {
		if requiredClass != nil {
			if _, exists := scMap[*requiredClass]; !exists {
				return fmt.Errorf("required storage class '%s' not found", *requiredClass)
			}
		}
	}
	return nil
}

func validateStateStorageUpdate(prev, ais *aisv1.AIStore) error {
	// We can't change volumeClaimTemplates in the statefulset, and therefore can't migrate to a state storage PVC
	// or change the storage class of an existing PVC. However, we can migrate to and from other storage methods.
	if !equality.Semantic.DeepEqual(ais.Spec.StateStoragePVCStorageClass(), prev.Spec.StateStoragePVCStorageClass()) && ais.Spec.StateStoragePVCStorageClass() != nil {
		return errCannotUpdateSpec("stateStorage.pvc.storageClass")
	}
	return nil
}

// SetupAIStoreWebhookWithManager registers the AIStore validating webhook with the manager.
func SetupAIStoreWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &aisv1.AIStore{}).
		WithValidator(&AIStoreWebhook{Client: mgr.GetClient()}).
		Complete()
}

// errors
func errCannotUpdateSpec(specName string, diff ...string) error {
	if len(diff) > 0 {
		return fmt.Errorf("cannot update spec %q for an existing cluster, diff: [%s]", specName, strings.Join(diff, ", "))
	}
	return fmt.Errorf("cannot update spec %q for an existing cluster", specName)
}
