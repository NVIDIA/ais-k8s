/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package webhook

import (
	"context"
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var author = authenticationv1.UserInfo{
	Username: "alice",
	UID:      "alice-uid",
	Groups:   []string{"system:authenticated", "tenant-admins"},
	Extra:    map[string]authenticationv1.ExtraValue{"tenant": {"acme"}},
}

// sarClient answers SubjectAccessReviews with a fixed decision and keeps the review it was given.
// The embedded nil client panics on any other call, which AuthorizeGet must not make.
type sarClient struct {
	client.Client
	allowed   bool
	createErr error
	submitted *authorizationv1.SubjectAccessReview
}

func (c *sarClient) Create(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
	sar, ok := obj.(*authorizationv1.SubjectAccessReview)
	if !ok {
		return fmt.Errorf("unexpected object %T", obj)
	}
	c.submitted = sar.DeepCopy()
	if c.createErr != nil {
		return c.createErr
	}
	sar.Status.Allowed = c.allowed
	sar.Status.Denied = !c.allowed
	return nil
}

func authorContext(user authenticationv1.UserInfo) context.Context {
	return admission.NewContextWithRequest(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: user},
	})
}

func secretAttrs() *authorizationv1.ResourceAttributes {
	return &authorizationv1.ResourceAttributes{Resource: "secrets", Namespace: "tenant", Name: "creds"}
}

func TestAuthorizeGet(t *testing.T) {
	path := field.NewPath("spec", "auth", "usernamePassword")

	for _, tt := range []struct {
		name string
		// ctx defaults to an admission request submitted by author
		ctx       context.Context
		allowed   bool
		createErr error
		// wantForbidden expects a field error naming the user and the referenced resource
		wantForbidden bool
		// wantErrMsg expects an internal error containing it
		wantErrMsg string
		// wantNoReview expects that no review was submitted
		wantNoReview bool
	}{
		{
			name:    "allows an authorized user",
			allowed: true,
		},
		{
			name:          "denies an unauthorized user",
			wantForbidden: true,
		},
		{
			name:         "fails without an admission request to identify the user",
			ctx:          context.Background(),
			allowed:      true,
			wantErrMsg:   "cannot authorize secrets reference",
			wantNoReview: true,
		},
		{
			name:       "fails when the review cannot be created",
			allowed:    true,
			createErr:  errors.New("apiserver unavailable"),
			wantErrMsg: `authorizing secrets "creds" in namespace "tenant"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := tt.ctx
			if ctx == nil {
				ctx = authorContext(author)
			}
			c := &sarClient{allowed: tt.allowed, createErr: tt.createErr}
			attrs := secretAttrs()

			fieldErr, err := AuthorizeGet(ctx, c, path, attrs)

			if tt.wantErrMsg == "" {
				g.Expect(err).NotTo(HaveOccurred())
			} else {
				g.Expect(apierrors.IsInternalError(err)).To(BeTrue())
				g.Expect(err).To(MatchError(ContainSubstring(tt.wantErrMsg)))
			}
			if tt.wantForbidden {
				g.Expect(fieldErr).NotTo(BeNil())
				g.Expect(fieldErr.Type).To(Equal(field.ErrorTypeForbidden))
				g.Expect(fieldErr.Field).To(Equal(path.String()))
				g.Expect(fieldErr.Error()).To(ContainSubstring(
					`user "alice" is not authorized to get secrets resource "creds" in namespace "tenant"`))
			} else {
				g.Expect(fieldErr).To(BeNil())
			}
			g.Expect(attrs).To(Equal(secretAttrs()), "caller attributes must not be modified")

			if tt.wantNoReview {
				g.Expect(c.submitted).To(BeNil())
				return
			}
			// The review must carry the submitting user's full identity, otherwise access is decided for the wrong subject
			g.Expect(c.submitted.Spec).To(Equal(authorizationv1.SubjectAccessReviewSpec{
				User:   author.Username,
				UID:    author.UID,
				Groups: author.Groups,
				Extra:  map[string]authorizationv1.ExtraValue{"tenant": {"acme"}},
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Verb:      "get",
					Resource:  "secrets",
					Namespace: "tenant",
					Name:      "creds",
				},
			}))
		})
	}
}
