/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"net/url"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func (p *AIStoreAuthProfile) ValidateSpec() error {
	allErrs := p.validateServiceURL()
	allErrs = append(allErrs, p.validateAuthMethod()...)
	allErrs = append(allErrs, p.validateTokenExchangeEndpoint()...)
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		GroupVersion.WithKind("AIStoreAuthProfile").GroupKind(),
		p.Name,
		allErrs,
	)
}

func (p *AIStoreAuthProfile) validateAuthMethod() field.ErrorList {
	if (p.Spec.UsernamePassword == nil) == (p.Spec.TokenExchange == nil) {
		return field.ErrorList{field.Invalid(
			field.NewPath("spec"),
			nil,
			"exactly one of usernamePassword or tokenExchange must be specified",
		)}
	}
	return nil
}

func (p *AIStoreAuthProfile) validateServiceURL() field.ErrorList {
	path := field.NewPath("spec", "serviceURL")
	u, err := url.Parse(p.Spec.ServiceURL)
	if err != nil {
		return field.ErrorList{field.Invalid(path, p.Spec.ServiceURL, err.Error())}
	}
	if u.Host == "" {
		return field.ErrorList{field.Invalid(path, p.Spec.ServiceURL, "must include a host")}
	}
	if u.Path != "" && u.Path != "/" {
		return field.ErrorList{field.Invalid(path, p.Spec.ServiceURL, "must not include a path")}
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return field.ErrorList{field.Invalid(path, p.Spec.ServiceURL, "must not include query or fragment")}
	}
	switch u.Scheme {
	case "https", "http":
		return nil
	default:
		return field.ErrorList{field.Invalid(path, p.Spec.ServiceURL, "scheme must be http or https")}
	}
}

func (p *AIStoreAuthProfile) validateTokenExchangeEndpoint() field.ErrorList {
	if p.Spec.TokenExchange == nil {
		return nil
	}
	path := field.NewPath("spec", "tokenExchange", "endpoint")
	endpoint := p.TokenExchangeEndpoint()
	u, err := url.Parse(endpoint)
	if err != nil {
		return field.ErrorList{field.Invalid(path, endpoint, err.Error())}
	}
	if u.Scheme != "" || u.Host != "" {
		return field.ErrorList{field.Invalid(path, endpoint, "must be a path, not a URL")}
	}
	if !strings.HasPrefix(u.Path, "/") || u.RawQuery != "" || u.Fragment != "" {
		return field.ErrorList{field.Invalid(path, endpoint, "must be an absolute path without query or fragment")}
	}
	return nil
}
