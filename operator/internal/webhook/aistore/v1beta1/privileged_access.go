/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1beta1

import (
	"context"
	"slices"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/resources/aistore/proxy"
	"github.com/ais-operator/internal/resources/aistore/target"
	webhookcmn "github.com/ais-operator/internal/webhook"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/pod-security-admission/api"
	"k8s.io/pod-security-admission/policy"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// privilegedAccessSubresource is the virtual aistores subresource that RBAC grants to allow privileged pods.
	privilegedAccessSubresource = "privileged"
	privilegedAccessVerb        = "use"
	privilegedAccessDocURL      = "https://github.com/NVIDIA/ais-k8s/blob/main/docs/privileged_access.md"
)

// The version is pinned so a library upgrade does not change who needs `aistores/privileged`.
var privilegedPolicy = api.LevelVersion{Level: api.LevelBaseline, Version: api.MajorMinorVersion(1, 35)}

// podSecurityEvaluator is built once at startup.
var podSecurityEvaluator = func() policy.Evaluator {
	// Host ports are exempt because they give no access beyond what the editor role already grants.
	checks := slices.DeleteFunc(policy.DefaultChecks(), func(c policy.Check) bool { return c.ID == policy.CheckHostPorts().ID })
	evaluator, err := policy.NewEvaluator(checks, nil)
	// The check set is static, so an error is a programming mistake.
	utilruntime.Must(err)
	return evaluator
}()

// podViolations lists how the proxy and target pods that the spec creates violate privilegedPolicy.
func podViolations(ais *aisv1.AIStore) []string {
	var violations []string
	for _, pod := range []struct {
		daemonType string
		template   *corev1.PodTemplateSpec
	}{
		{aisapc.Proxy, &proxy.NewProxyStatefulSet(ais, 1).Spec.Template},
		{aisapc.Target, &target.NewTargetSS(ais, 1).Spec.Template},
	} {
		results := podSecurityEvaluator.EvaluatePod(privilegedPolicy, &pod.template.ObjectMeta, &pod.template.Spec)
		if agg := policy.AggregateCheckResults(results); !agg.Allowed {
			violations = append(violations, pod.daemonType+" pods: "+agg.ForbiddenDetail())
		}
	}
	return violations
}

// validatePrivilegedAccess rejects the spec when the user may not use `aistores/privileged` and the pods that
// the spec creates violate the Pod Security Standards baseline policy.
func (aisw *AIStoreWebhook) validatePrivilegedAccess(ctx context.Context, ais *aisv1.AIStore) error {
	violations := podViolations(ais)
	if len(violations) == 0 {
		return nil
	}

	fieldErr, err := webhookcmn.Authorize(ctx, aisw.Client, privilegedAccessVerb, field.NewPath("spec"), &authorizationv1.ResourceAttributes{
		Group:       aisv1.GroupVersion.Group,
		Version:     aisv1.GroupVersion.Version,
		Resource:    "aistores",
		Subresource: privilegedAccessSubresource,
		Namespace:   ais.Namespace,
		Name:        ais.Name,
	})
	if err != nil || fieldErr == nil {
		return err
	}
	req, _ := admission.RequestFromContext(ctx)
	webhooklog.Info("Rejected spec: it requires aistores/privileged access and the user does not have it",
		"name", ais.Name, "namespace", ais.Namespace, "user", req.UserInfo.Username, "groups", req.UserInfo.Groups, "violations", violations)
	fieldErr.Detail += ". Violates Pod Security Standards baseline policy: " + strings.Join(violations, "; ") + ". See " + privilegedAccessDocURL
	return apiInvalidError(ais, fieldErr)
}
