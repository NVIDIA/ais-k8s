/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package webhook contains shared webhook functions
package webhook

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AuthorizeGet verifies via SubjectAccessReview that the submitting user may get
// the object described by attrs.
func AuthorizeGet(
	ctx context.Context,
	c client.Client,
	path *field.Path,
	attrs *authorizationv1.ResourceAttributes,
) (*field.Error, error) {
	return Authorize(ctx, c, "get", path, attrs)
}

// Authorize verifies via SubjectAccessReview that the submitting user may perform
// verb on the object described by attrs.
func Authorize(
	ctx context.Context,
	c client.Client,
	verb string,
	path *field.Path,
	attrs *authorizationv1.ResourceAttributes,
) (*field.Error, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, apierrors.NewInternalError(
			fmt.Errorf("cannot authorize %s: %w", describeResource(attrs), err),
		)
	}
	userInfo := req.UserInfo
	verbAttrs := attrs.DeepCopy()
	verbAttrs.Verb = verb

	extra := make(map[string]authorizationv1.ExtraValue, len(userInfo.Extra))
	for k, v := range userInfo.Extra {
		extra[k] = authorizationv1.ExtraValue(v)
	}

	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:               userInfo.Username,
			UID:                userInfo.UID,
			Groups:             userInfo.Groups,
			Extra:              extra,
			ResourceAttributes: verbAttrs,
		},
	}
	if err := c.Create(ctx, sar); err != nil {
		return nil, apierrors.NewInternalError(
			fmt.Errorf("authorizing %s: %w", describeResource(attrs), err),
		)
	}
	if !sar.Status.Allowed {
		msg := fmt.Sprintf("Denied unauthorized access to %s", describeResource(attrs))
		logf.FromContext(ctx).V(1).Info(msg, "user", userInfo.Username, "groups", userInfo.Groups)
		return field.Forbidden(path, fmt.Sprintf(
			"user %q is not authorized to %s %s",
			userInfo.Username, verb, describeResource(attrs),
		)), nil
	}
	return nil, nil
}

// describeResource names the reviewed object, omitting the namespace for cluster-scoped resources.
func describeResource(attrs *authorizationv1.ResourceAttributes) string {
	if attrs.Namespace == "" {
		return fmt.Sprintf("%s resource %q", attrs.Resource, attrs.Name)
	}
	return fmt.Sprintf("%s resource %q in namespace %q", attrs.Resource, attrs.Name, attrs.Namespace)
}
