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
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/go-test/deep"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
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
	return aisw.validateSpec(ctx, ais, nil)
}

// ValidateUpdate implements admission.Validator.
func (aisw *AIStoreWebhook) ValidateUpdate(ctx context.Context, prev, ais *aisv1.AIStore) (admission.Warnings, error) {
	webhooklog.WithValues("name", ais.Name, "namespace", ais.Namespace).Info("Validate update")
	warnings, err := aisw.validateSpec(ctx, ais, prev)
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
func (aisw *AIStoreWebhook) validateSpec(ctx context.Context, ais, prev *aisv1.AIStore) (admission.Warnings, error) {
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

	err = aisw.validateAuthSecretAccess(ctx, ais, prev)
	return allWarnings, err
}

func authSecretNamespace(ais *aisv1.AIStore, up *aisv1.UsernamePasswordAuth) string {
	if up.SecretNamespace != nil {
		return *up.SecretNamespace
	}
	return ais.Namespace
}

// shouldVerifyAuthSecret checks if we must verify user access to the provided auth secret reference
func shouldVerifyAuthSecret(prev, ais *aisv1.AIStore) bool {
	// Nothing to verify
	if ais.Spec.Auth == nil || ais.Spec.Auth.UsernamePassword == nil {
		return false
	}
	// If no previous entry, we must verify access
	if prev == nil || prev.Spec.Auth == nil || prev.Spec.Auth.UsernamePassword == nil {
		return true
	}
	// Only require SubjectAccessReview if the reference changed from previous
	prevUp := prev.Spec.Auth.UsernamePassword
	currUp := ais.Spec.Auth.UsernamePassword
	if prevUp.SecretName != currUp.SecretName {
		return true
	}
	return authSecretNamespace(prev, prevUp) != authSecretNamespace(ais, currUp)
}

// validateAuthSecretAccess verifies the submitting user can get the auth credentials
// Secret referenced by spec.auth.usernamePassword. The check runs at admission time
// on create and on update only when the secret reference changes.
func (aisw *AIStoreWebhook) validateAuthSecretAccess(ctx context.Context, ais, prev *aisv1.AIStore) error {
	if !shouldVerifyAuthSecret(prev, ais) {
		return nil
	}
	up := ais.Spec.Auth.UsernamePassword
	secretNS := authSecretNamespace(ais, up)

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return fmt.Errorf("cannot authorize auth secret reference: %w", err)
	}
	userInfo := req.UserInfo

	extra := make(map[string]authorizationv1.ExtraValue, len(userInfo.Extra))
	for k, v := range userInfo.Extra {
		extra[k] = authorizationv1.ExtraValue(v)
	}

	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   userInfo.Username,
			UID:    userInfo.UID,
			Groups: userInfo.Groups,
			Extra:  extra,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: secretNS,
				Verb:      "get",
				Group:     "",
				Resource:  "secrets",
				Name:      up.SecretName,
			},
		},
	}
	if err := aisw.Client.Create(ctx, sar); err != nil {
		return fmt.Errorf("failed to authorize auth secret %q in namespace %q: %w", up.SecretName, secretNS, err)
	}
	if !sar.Status.Allowed {
		return errUnauthorizedAuthSecret(userInfo.Username, up.SecretName, secretNS)
	}
	return nil
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

func errUnauthorizedAuthSecret(user, secretName, secretNamespace string) error {
	return fmt.Errorf("user %q is not authorized to get Secret %q in namespace %q referenced by spec.auth.usernamePassword", user, secretName, secretNamespace)
}
