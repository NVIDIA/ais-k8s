/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"fmt"
	"net/url"
	"strings"
)

func (p *AIStoreAuthProfile) ValidateSpec() error {
	if err := p.validateServiceURL(); err != nil {
		return err
	}
	return p.validateTokenExchangeEndpoint()
}

func (p *AIStoreAuthProfile) validateServiceURL() error {
	u, err := url.Parse(p.Spec.ServiceURL)
	if err != nil {
		return fmt.Errorf("spec.serviceURL is invalid: %w", err)
	}
	if u.Host == "" {
		return fmt.Errorf("spec.serviceURL must include a host")
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("spec.serviceURL must not include a path")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("spec.serviceURL must not include query or fragment")
	}
	switch u.Scheme {
	case "https", "http":
		return nil
	default:
		return fmt.Errorf("spec.serviceURL scheme must be http or https")
	}
}

func (p *AIStoreAuthProfile) validateTokenExchangeEndpoint() error {
	if p.Spec.TokenExchange == nil {
		return nil
	}
	u, err := url.Parse(p.TokenExchangeEndpoint())
	if err != nil {
		return fmt.Errorf("spec.tokenExchange.endpoint is invalid: %w", err)
	}
	if u.Scheme != "" || u.Host != "" {
		return fmt.Errorf("spec.tokenExchange.endpoint must be a path, not a URL")
	}
	if !strings.HasPrefix(u.Path, "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("spec.tokenExchange.endpoint must be an absolute path without query or fragment")
	}
	return nil
}
