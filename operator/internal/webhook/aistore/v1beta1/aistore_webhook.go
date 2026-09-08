/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package v1beta1 contains admission webhooks for the ais.nvidia.com/v1beta1 API group.
package v1beta1

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
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

// validateSpec runs the spec-only validations defined on the AIStore type along with
// webhook-only validations that require admission or cluster context.
func (aisw *AIStoreWebhook) validateSpec(ctx context.Context, prev, ais *aisv1.AIStore) (admission.Warnings, error) {
	// Writes with no changes must not invalidate the spec so the operator can still patch finalizers and annotations.
	if prev != nil && equality.Semantic.DeepEqual(&prev.Spec, &ais.Spec) {
		warnings, _ := ais.ValidateSpec(ctx)
		return warnings, nil
	}

	if err := validateDeprecatedStateStorage(ais); err != nil {
		return nil, err
	}

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

	err = validateRequiredAudiences(ais)
	if err != nil {
		return allWarnings, err
	}

	err = aisw.validateAuthProfile(ctx, ais)
	return allWarnings, err
}

// requiredAudiencesPath locates auth.required_claims.aud in an AIStore spec.
func requiredAudiencesPath() *field.Path {
	return field.NewPath("spec", "configToUpdate", "auth", "required_claims", "aud")
}

// isClusterAudience reports whether aud has the <namespace>/<name> form the operator uses to
// identify an AIStore cluster.
func isClusterAudience(aud string) bool {
	ns, name, found := strings.Cut(aud, "/")
	return found && len(validation.IsDNS1123Label(ns)) == 0 && len(validation.IsDNS1123Subdomain(name)) == 0
}

// validateRequiredAudiences rejects an audience that could potentially identify an AIStore cluster
// other than this one.
func validateRequiredAudiences(ais *aisv1.AIStore) error {
	own := ais.TokenAudience()
	for i, aud := range ais.RequiredAudiences() {
		if aud == own || !isClusterAudience(aud) {
			continue
		}
		return apiInvalidError(ais, field.Invalid(requiredAudiencesPath().Index(i), aud,
			fmt.Sprintf("an audience in <namespace>/<name> form identifies an AIStore cluster. Only %q is allowed", own)))
	}
	return nil
}

// validateAuthProfile fetches and validates the referenced authentication profile
func (aisw *AIStoreWebhook) validateAuthProfile(ctx context.Context, ais *aisv1.AIStore) error {
	profRefPath := field.NewPath("spec", "auth", "profileRef")
	ref := ais.GetAuthProfileRef()
	if ref == nil {
		return nil
	}
	if err := aisw.validateAuthProfileRef(ctx, ais, profRefPath, ref.Name); err != nil {
		return err
	}
	prof, err := aisw.getAuthProfile(ctx, ais, profRefPath, ref.Name)
	if err != nil {
		return err
	}
	return validateTokenExchangeAud(ais, prof)
}

// validateAuthProfileRef checks the editor has access to the referenced spec.auth.profileRef
func (aisw *AIStoreWebhook) validateAuthProfileRef(ctx context.Context, ais *aisv1.AIStore, path *field.Path, refName string) error {
	// Requires "use" on the referenced AIStoreAuthProfile
	return aisw.authorize(ctx, ais, "use", path,
		&authorizationv1.ResourceAttributes{
			Group:    authv1alpha1.GroupVersion.Group,
			Version:  authv1alpha1.GroupVersion.Version,
			Resource: "aistoreauthprofiles",
			Name:     refName,
		})
}

// getAuthProfile fetches the referenced AIStoreAuthProfile using operator permissions
func (aisw *AIStoreWebhook) getAuthProfile(ctx context.Context, ais *aisv1.AIStore, path *field.Path, refName string) (*authv1alpha1.AIStoreAuthProfile, error) {
	prof := &authv1alpha1.AIStoreAuthProfile{}
	if err := aisw.Client.Get(ctx, client.ObjectKey{Name: refName}, prof); err != nil {
		if apierrors.IsNotFound(err) {
			apiErr := field.Invalid(path, refName, "referenced AIStoreAuthProfile does not exist")
			return nil, apiInvalidError(ais, apiErr)
		}
		return nil, apierrors.NewInternalError(
			fmt.Errorf("checking AIStoreAuthProfile %q: %w", refName, err),
		)
	}
	return prof, nil
}

// validateTokenExchangeAud requires the cluster's own token audience in auth.required_claims.aud
// when the referenced profile uses token exchange.
func validateTokenExchangeAud(ais *aisv1.AIStore, prof *authv1alpha1.AIStoreAuthProfile) error {
	if prof.Spec.TokenExchange == nil {
		return nil
	}
	// AIS only validates the claim when at least one audience is listed
	required := ais.RequiredAudiences()
	if len(required) == 0 {
		return nil
	}
	if slices.Contains(required, ais.TokenAudience()) {
		return nil
	}
	apiErr := field.Invalid(requiredAudiencesPath(), required, fmt.Sprintf(
		"must include %q when the referenced AIStoreAuthProfile uses token exchange", ais.TokenAudience()))
	return apiInvalidError(ais, apiErr)
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
	return apiInvalidError(ais, fieldErr)
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
	prev.AISContainerSecurityContext = spec.AISContainerSecurityContext
	prev.AutoScaleConf = spec.AutoScaleConf
	prev.PVCRetentionPolicy = spec.PVCRetentionPolicy
	prev.Probes = spec.Probes
	prev.Tolerations = spec.Tolerations
	if spec.ServicePort == nil { //nolint:staticcheck // clearing the deprecated option must be allowed
		prev.ServicePort = nil //nolint:staticcheck // clearing the deprecated option must be allowed
	}
	// Retuning an existing LoadBalancer is safe; toggling external access on or off is not.
	if prev.ExternalAccess != nil && spec.ExternalAccess != nil {
		prev.ExternalAccess = spec.ExternalAccess
	}
}

// allowProxyPortUpdates copies proxy port fields whose port number is unchanged once defaults are
// applied, so an existing cluster can drop port fields that already match the defaults.
func allowProxyPortUpdates(prev, ais *aisv1.AIStore) {
	prevSpec, spec := &prev.Spec.ProxySpec.ServiceSpec, &ais.Spec.ProxySpec.ServiceSpec
	if samePort(prev.ProxyPublicPort(), ais.ProxyPublicPort()) {
		prevSpec.PublicPort = spec.PublicPort
	}
	if samePort(prev.ProxyIntraControlPort(), ais.ProxyIntraControlPort()) {
		prevSpec.IntraControlPort = spec.IntraControlPort
	}
	if samePort(prev.ProxyIntraDataPort(), ais.ProxyIntraDataPort()) {
		prevSpec.IntraDataPort = spec.IntraDataPort
	}
}

// allowTargetPortUpdates copies target port fields whose port number is unchanged once defaults are
// applied, so an existing cluster can drop port fields that already match the defaults.
func allowTargetPortUpdates(prev, ais *aisv1.AIStore) {
	prevSpec, spec := &prev.Spec.TargetSpec.ServiceSpec, &ais.Spec.TargetSpec.ServiceSpec
	if samePort(prev.TargetPublicPort(), ais.TargetPublicPort()) {
		prevSpec.PublicPort = spec.PublicPort
	}
	if samePort(prev.TargetIntraControlPort(), ais.TargetIntraControlPort()) {
		prevSpec.IntraControlPort = spec.IntraControlPort
	}
	if samePort(prev.TargetIntraDataPort(), ais.TargetIntraDataPort()) {
		prevSpec.IntraDataPort = spec.IntraDataPort
	}
}

func samePort(a, b intstr.IntOrString) bool {
	return a.IntValue() == b.IntValue()
}

func validateProxyUpdate(prev, ais *aisv1.AIStore) error {
	allowDaemonSpecUpdates(&prev.Spec.ProxySpec, &ais.Spec.ProxySpec)
	allowProxyPortUpdates(prev, ais)
	if !equality.Semantic.DeepEqual(ais.Spec.ProxySpec, prev.Spec.ProxySpec) {
		diff := deep.Equal(ais.Spec.ProxySpec, prev.Spec.ProxySpec)
		webhooklog.Info(fmt.Sprintf("Differences found in proxy spec: [%s]", strings.Join(diff, ", ")))
		return errCannotUpdateSpec("proxySpec", diff...)
	}
	return nil
}

func validateTargetUpdate(prev, ais *aisv1.AIStore) error {
	allowDaemonSpecUpdates(&prev.Spec.TargetSpec.DaemonSpec, &ais.Spec.TargetSpec.DaemonSpec)
	allowTargetPortUpdates(prev, ais)
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

// validateDeprecatedStateStorage rejects the state storage options that were replaced by
// spec.stateStorage.
func validateDeprecatedStateStorage(ais *aisv1.AIStore) error {
	if msgs := ais.Spec.DeprecatedStateStorageMessages(); len(msgs) > 0 {
		return errors.New(strings.Join(msgs, "; "))
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
		WithValidator(&AIStoreWebhook{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// errors
func errCannotUpdateSpec(specName string, diff ...string) error {
	if len(diff) > 0 {
		return fmt.Errorf("cannot update spec %q for an existing cluster, diff: [%s]", specName, strings.Join(diff, ", "))
	}
	return fmt.Errorf("cannot update spec %q for an existing cluster", specName)
}

func apiInvalidError(ais *aisv1.AIStore, err *field.Error) *apierrors.StatusError {
	return apierrors.NewInvalid(
		aisv1.GroupVersion.WithKind("AIStore").GroupKind(),
		ais.Name,
		field.ErrorList{err},
	)
}
