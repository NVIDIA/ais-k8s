/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	certres "github.com/ais-operator/internal/resources/certificates"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) reconcileTLSCertificate(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	if !authn.UseTLSCertificate() {
		return r.deleteCertificate(ctx, authn)
	}
	externalEndpoints, err := r.loadBalancerEndpoints(ctx, authn)
	if err != nil {
		return err
	}
	certificate := authnres.NewCertificate(ctx, authn, externalEndpoints)
	if err := r.client.Apply(ctx, certificate); err != nil {
		return err
	}
	logf.FromContext(ctx).V(1).Info("AuthN TLS Certificate applied", "name", authnres.CertificateName(authn))
	return nil
}

// deleteCertificate removes the Certificate at the name the operator derives for this CR.
func (r *Reconciler) deleteCertificate(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	_, err := r.client.DeleteResourceIfExists(ctx, authnres.TLSCertificate(authn))
	return err
}

func (r *Reconciler) loadBalancerEndpoints(ctx context.Context, authn *authv1alpha1.AIStoreAuth) ([]string, error) {
	if authn.Spec.ExternalAccess == nil || authn.Spec.ExternalAccess.LoadBalancer == nil {
		return nil, nil
	}
	svc, err := r.client.GetService(ctx, authnres.LoadBalancerServiceNSName(authn))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return certres.LoadBalancerEndpoints(*svc), nil
}
