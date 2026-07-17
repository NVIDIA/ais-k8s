/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1beta1

import (
	"context"
	"fmt"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ValidateSpec runs the spec-only validations that do not require cluster access.
func (ais *AIStore) ValidateSpec(_ context.Context) (admission.Warnings, error) {
	var allWarnings admission.Warnings
	validations := []func() (admission.Warnings, error){
		ais.validateSize,
		ais.validateStateStorage,
		ais.validateShutdownWithEmptyDir,
		ais.validateAutoScaling,
		ais.validateServiceSpec,
		ais.validateCleanupConfig,
		ais.validateTLSCertPaths,
		ais.validateSafeDecommission,
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
	return fmt.Errorf("AIS spec stateStorage must define exactly one of hostPath, pvc, or emptyDir")
}

func errUndefinedNodeSelector(spec string) error {
	return fmt.Errorf("missing nodeSelector for %s; nodeSelector is required when autoScale is enabled", spec)
}
