/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1beta1

import (
	"context"
	"fmt"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ValidateSpec runs the spec-only validations that do not require cluster access.
func (ais *AIStore) ValidateSpec(_ context.Context) (admission.Warnings, error) {
	var allWarnings admission.Warnings
	validations := []func() (admission.Warnings, error){
		ais.validateDeprecatedFields,
		ais.validateSize,
		ais.validateStateStorage,
		ais.validateShutdownWithEmptyDir,
		ais.validateAutoScaling,
		ais.validateServiceSpec,
		ais.validateExternalAccess,
		ais.validateHostPort,
		ais.validateCleanupConfig,
		ais.validateTLSCertPaths,
		ais.validatePublicTLSCertPaths,
		ais.validateSafeDecommission,
		ais.validateAuthConfig,
		ais.validateAuth,
	}

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
		if !ais.Spec.hasExactlyOneStateStorageMode() {
			return nil, errInvalidStateStorage()
		}
		return nil, nil
	}
	if ais.Spec.StateStorageClass == nil && ais.Spec.HostpathPrefix == nil {
		return nil, errUndefinedStateStorage()
	}
	return nil, nil
}

func (ais *AIStore) validateShutdownWithEmptyDir() (admission.Warnings, error) {
	if ais.Spec.UsesStateEmptyDir() && ais.ShouldBeShutdown() {
		return nil, fmt.Errorf("shutdownCluster cannot be enabled when stateStorage.emptyDir is used; emptyDir state is ephemeral and is lost when the cluster is shut down")
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
	if s.StateStorage.EmptyDir != nil {
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

// validateSafeDecommission warns when rebalance is disabled while using scaleDownMode safe_decommission.
func (ais *AIStore) validateSafeDecommission() (admission.Warnings, error) {
	if !ais.Spec.TargetSpec.SafeDecommissionOnScaleDown() {
		return nil, nil
	}
	if !ais.Spec.ConfigToUpdate.RebalanceEnabled() {
		return admission.Warnings{fmt.Sprintf("scaleDownMode is %q but rebalance is disabled; enable configToUpdate.rebalance.enabled so target data is migrated on scale-down", ScaleDownModeSafeDecommission)}, nil
	}
	return nil, nil
}

// validateAuth rejects specs where AIS expects authenticated API calls but the operator has no
// configuration from which to obtain a token, and warns on the reverse mismatch.
func (ais *AIStore) validateAuth() (admission.Warnings, error) {
	if ais.GetAuthProfileRef() != nil {
		if !ais.Spec.ConfigToUpdate.RequiresClientAuth() {
			return admission.Warnings{"spec.auth.profileRef is set but AIS is not configured to authenticate client requests"}, nil
		}
		return nil, nil
	}
	if ais.Spec.Auth != nil {
		return nil, fmt.Errorf("spec.auth is empty; set spec.auth.profileRef")
	}
	if !ais.Spec.ConfigToUpdate.RequiresClientAuth() {
		return nil, nil
	}
	return nil, fmt.Errorf("provided spec configures AIS to authenticate client requests; set spec.auth.profileRef")
}

// validateAuthConfig rejects specs that contain incompatible AIS spec and config options for authentication
func (ais *AIStore) validateAuthConfig() (admission.Warnings, error) {
	if ais.Spec.AuthNSecretName != nil && ais.Spec.ConfigToUpdate.HasOIDCIssuers() {
		return nil, fmt.Errorf("spec.authNSecretName defines an HMAC signing secret and is incompatible with configured OIDC issuers in spec.configToUpdate")
	}
	return nil, nil
}

// validateDeprecatedFields warns on spec fields that are slated for removal.
func (ais *AIStore) validateDeprecatedFields() (admission.Warnings, error) {
	warnings := ais.Spec.DeprecatedStateStorageMessages()
	warnings = append(warnings, ais.Spec.DeprecatedPortMessages()...)
	return append(warnings, ais.Spec.ConfigToUpdate.DeprecatedAuthMessages()...), nil
}

func validatePorts(path *field.Path, ports *daemonPorts) field.ErrorList {
	var allErrs field.ErrorList
	appendPortErrs := func(name string, port intstr.IntOrString) {
		for _, msg := range validation.IsValidPortNum(port.IntValue()) {
			allErrs = append(allErrs, field.Invalid(path.Child(name), port.IntValue(), msg))
		}
	}
	if ports.service != nil {
		appendPortErrs("servicePort", *ports.service)
	}
	appendPortErrs("portPublic", ports.public)
	appendPortErrs("portIntraControl", ports.intraControl)
	appendPortErrs("portIntraData", ports.intraData)

	return allErrs
}

func (ais *AIStore) validateServiceSpec() (admission.Warnings, error) {
	allErrs := validatePorts(field.NewPath("spec", "proxySpec"), ais.proxyPorts())
	allErrs = append(allErrs, validatePorts(field.NewPath("spec", "targetSpec"), ais.targetPorts())...)

	return nil, allErrs.ToAggregate()
}

func (ais *AIStore) validateExternalAccess() (admission.Warnings, error) {
	ea := ais.Spec.TargetSpec.ExternalAccess
	if ea == nil || ea.LoadBalancer == nil || ea.LoadBalancer.Port == nil {
		return nil, nil
	}
	// Reject a LoadBalancer port on targets.
	// Proxies redirect clients to the target address AIS advertises, which is always portPublic.
	return nil, field.Invalid(
		field.NewPath("spec", "targetSpec", "externalAccess", "loadBalancer", "port"),
		*ea.LoadBalancer.Port,
		"not supported; target LoadBalancers must listen on spec.targetSpec.portPublic",
	)
}

func (ais *AIStore) validateHostPort() (admission.Warnings, error) {
	hostPort := ais.Spec.TargetSpec.HostPort
	if !ais.UseHostNetwork() || hostPort == nil {
		return nil, nil
	}
	publicPort := ais.TargetPublicPort()
	// K8s requires containerPort (set from publicPort) to equal hostPort when hostNetwork is true.
	if publicPort.IntValue() == int(*hostPort) {
		return nil, nil
	}
	return nil, field.Invalid(
		field.NewPath("spec", "targetSpec", "hostPort"),
		*hostPort,
		"must match spec.targetSpec.portPublic when hostNetwork is enabled",
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
	conflicts := getConfiguredCertPaths(http.Certificate, http.CertKey, http.ClientCA)
	if len(conflicts) == 0 {
		return nil, nil
	}
	return nil, fmt.Errorf("configToUpdate.net.http.[%s] cannot be set together with spec.tls; the operator manages cert paths under /var/certs", strings.Join(conflicts, ","))
}

// validatePublicTLSCertPaths rejects specs that set both spec.tls.public and any of the cert path
// fields in configToUpdate.net.http.pub, since the operator manages those paths under
// /var/certs-pub and would silently override them.
func (ais *AIStore) validatePublicTLSCertPaths() (admission.Warnings, error) {
	if !ais.HasPublicTLS() || ais.Spec.ConfigToUpdate == nil || ais.Spec.ConfigToUpdate.Net == nil || ais.Spec.ConfigToUpdate.Net.HTTP == nil {
		return nil, nil
	}
	pub := ais.Spec.ConfigToUpdate.Net.HTTP.Pub
	if pub == nil {
		return nil, nil
	}
	conflicts := getConfiguredCertPaths(pub.Certificate, pub.CertKey, pub.ClientCA)
	if len(conflicts) == 0 {
		return nil, nil
	}
	return nil, fmt.Errorf("configToUpdate.net.http.pub.[%s] cannot be set together with spec.tls.public; the operator manages cert paths under /var/certs-pub", strings.Join(conflicts, ","))
}

// getConfiguredCertPaths names the cert path options that are set. The operator writes every one of them for
// each TLS section it manages, so both sections reject the same set.
func getConfiguredCertPaths(certificate, certKey, clientCA *string) []string {
	var set []string
	if certificate != nil {
		set = append(set, "server_crt")
	}
	if certKey != nil {
		set = append(set, "server_key")
	}
	if clientCA != nil {
		set = append(set, "client_ca_tls")
	}
	return set
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

// errors
func errInvalidClusterSize(size *int32) error {
	if size == nil {
		return fmt.Errorf("cluster size is not specified")
	}
	return fmt.Errorf("invalid cluster size %d, should be at least 1 or -1 for autoScaling", *size)
}

func errInvalidDaemonSize(size int32, daeType string) error {
	return fmt.Errorf("invalid %s daemon size %d, should be at least 1", daeType, size)
}

func errUndefinedStateStorage() error {
	return fmt.Errorf("AIS spec does not define stateStorage. Set stateStorage.hostPath, stateStorage.pvc, or stateStorage.emptyDir")
}

func errInvalidStateStorage() error {
	return fmt.Errorf("AIStore spec.stateStorage must define exactly one of hostPath, pvc, or emptyDir")
}

func errUndefinedNodeSelector(spec string) error {
	return fmt.Errorf("missing nodeSelector for %s; nodeSelector is required when autoScale is enabled", spec)
}
