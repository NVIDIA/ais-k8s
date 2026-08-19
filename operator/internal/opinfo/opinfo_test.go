/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package opinfo

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	operatorNamespace = "ais-operator-system"
	operatorSA        = "ais-operator-controller-manager"
	testClusterDomain = "k8s.example.com"
)

// reviewedAs answers every SelfSubjectReview with the given username.
func reviewedAs(username string) client.Client {
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			review, ok := obj.(*authenticationv1.SelfSubjectReview)
			if !ok {
				return c.Create(ctx, obj, opts...)
			}
			review.Status.UserInfo = authenticationv1.UserInfo{Username: username}
			return nil
		},
	}).Build()
}

var _ = Describe("selfServiceAccountName", func() {
	var ctx = context.TODO()

	It("should return the ServiceAccount the client authenticates as", func() {
		sa, err := selfServiceAccountName(ctx,
			reviewedAs("system:serviceaccount:"+operatorNamespace+":"+operatorSA))
		Expect(err).NotTo(HaveOccurred())
		Expect(sa).To(Equal(types.NamespacedName{Namespace: operatorNamespace, Name: operatorSA}))
	})

	It("should fail when the client does not authenticate as a ServiceAccount", func() {
		_, err := selfServiceAccountName(ctx, reviewedAs("kubernetes-admin"))
		Expect(err).To(MatchError(ContainSubstring("is not a ServiceAccount")))
	})

	It("should fail when the ServiceAccount username has no namespace and name", func() {
		_, err := selfServiceAccountName(ctx, reviewedAs("system:serviceaccount:"+operatorNamespace))
		Expect(err).To(MatchError(ContainSubstring("cannot determine a ServiceAccount")))
	})

	It("should fail when the review is rejected", func() {
		c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
				return apierrors.NewForbidden(schema.GroupResource{
					Group: "authentication.k8s.io", Resource: "selfsubjectreviews",
				}, "", errors.New("not allowed"))
			},
		}).Build()

		_, err := selfServiceAccountName(ctx, c)
		Expect(err).To(MatchError(ContainSubstring("failed to review the operator's own identity")))
		Expect(apierrors.IsForbidden(err)).To(BeTrue(), "expected the API error to stay unwrappable")
	})
})

var _ = Describe("retriableReviewError", func() {
	var reviews = schema.GroupResource{Group: "authentication.k8s.io", Resource: "selfsubjectreviews"}

	DescribeTable("should retry an error the API server may answer differently",
		func(err error) {
			Expect(retriableReviewError(err)).To(BeTrue())
		},
		Entry("an unreachable API server",
			&url.Error{Op: "Post", URL: "https://10.96.0.1:443", Err: errors.New("connect: connection refused")}),
		Entry("a closed connection",
			&url.Error{Op: "Post", URL: "https://10.96.0.1:443", Err: io.ErrUnexpectedEOF}),
		Entry("a throttled request", apierrors.NewTooManyRequests("busy", 1)),
		Entry("an unavailable API server", apierrors.NewServiceUnavailable("no healthy backends")),
		Entry("a server-side timeout", apierrors.NewServerTimeout(reviews, "create", 1)),
		Entry("a request that did not complete", apierrors.NewTimeoutError("too slow", 1)),
		Entry("an internal error", apierrors.NewInternalError(errors.New("boom"))),
	)

	DescribeTable("should not retry an error that will not change",
		func(err error) {
			Expect(retriableReviewError(err)).To(BeFalse())
		},
		Entry("an unauthenticated client", apierrors.NewUnauthorized("no token")),
		Entry("a client without permission", apierrors.NewForbidden(reviews, "", errors.New("not allowed"))),
		Entry("an API server that does not serve the resource", apierrors.NewNotFound(reviews, "")),
		Entry("a cluster that does not know the kind", &meta.NoKindMatchError{
			GroupKind:        schema.GroupKind{Group: "authentication.k8s.io", Kind: "SelfSubjectReview"},
			SearchedVersions: []string{"v1"},
		}),
		Entry("an expired deadline",
			&url.Error{Op: "Post", URL: "https://10.96.0.1:443", Err: context.DeadlineExceeded}),
		Entry("a canceled request", context.Canceled),
	)
})

var _ = Describe("domainFromResolvConf", func() {
	DescribeTable("should take the domain from the search list",
		func(resolvConf, expected string) {
			domain, err := domainFromResolvConf(strings.NewReader(resolvConf))
			Expect(err).NotTo(HaveOccurred())
			Expect(domain).To(Equal(expected))
		},
		Entry("as kubelet writes it",
			"nameserver 10.96.0.10\n"+
				"search ais-operator-system.svc.cluster.local svc.cluster.local cluster.local\n"+
				"options ndots:5\n",
			"cluster.local"),
		Entry("a domain other than the default",
			"search ais-operator-system.svc.k8s.example.com svc.k8s.example.com k8s.example.com\n",
			"k8s.example.com"),
		Entry("fully qualified entries",
			"search ais-operator-system.svc.cluster.local. svc.cluster.local. cluster.local.\n",
			"cluster.local"),
		Entry("host search domains appended",
			"search svc.cluster.local cluster.local corp.example.com\n",
			"cluster.local"),
	)

	DescribeTable("should report no domain when the search list has no cluster entry",
		func(resolvConf string) {
			domain, err := domainFromResolvConf(strings.NewReader(resolvConf))
			Expect(err).NotTo(HaveOccurred())
			Expect(domain).To(BeEmpty())
		},
		Entry("a node's own configuration", "nameserver 10.0.0.1\nsearch corp.example.com\n"),
		Entry("only the namespace entry", "search ais-operator-system.svc.cluster.local\n"),
		Entry("a service entry naming no domain", "search svc.\n"),
		Entry("no search line", "nameserver 10.0.0.1\n"),
		Entry("nothing at all", ""),
	)
})

var _ = Describe("discoverClusterDomain", func() {
	It("should report no domain when the resolver configuration has no cluster entry", func() {
		resolvConf := filepath.Join(GinkgoT().TempDir(), "resolv.conf")
		Expect(os.WriteFile(resolvConf, []byte("nameserver 10.0.0.1\nsearch corp.example.com\n"), 0o600)).To(Succeed())

		domain, err := discoverClusterDomain(resolvConf)

		Expect(err).NotTo(HaveOccurred())
		Expect(domain).To(BeEmpty())
	})
})

var _ = Describe("resolveClusterDomain", func() {
	It("should prefer the configured domain over discovery", func() {
		GinkgoT().Setenv(clusterDomainEnv, testClusterDomain)

		domain, err := resolveClusterDomain(GinkgoLogr)

		Expect(err).NotTo(HaveOccurred())
		Expect(domain).To(Equal(testClusterDomain))
	})

	It("should fail when the configured domain is not a DNS domain", func() {
		GinkgoT().Setenv(clusterDomainEnv, "not_a_domain")

		_, err := resolveClusterDomain(GinkgoLogr)

		Expect(err).To(MatchError(ContainSubstring("not_a_domain")))
	})
})

var _ = Describe("Resolve", func() {
	BeforeEach(func() {
		// the host's resolver configuration is not the operator's
		GinkgoT().Setenv(clusterDomainEnv, testClusterDomain)

		domain, sa := clusterDomain, serviceAccount
		DeferCleanup(func() {
			clusterDomain, serviceAccount = domain, sa
		})
	})

	It("should publish both facts for the rest of the operator to read", func() {
		err := Resolve(context.TODO(),
			reviewedAs("system:serviceaccount:"+operatorNamespace+":"+operatorSA))

		Expect(err).NotTo(HaveOccurred())
		Expect(ClusterDomain()).To(Equal(testClusterDomain))
		Expect(ServiceAccount()).To(Equal(types.NamespacedName{
			Namespace: operatorNamespace,
			Name:      operatorSA,
		}))
	})

	It("should fail when the identity cannot be determined", func() {
		err := Resolve(context.TODO(), reviewedAs("kubernetes-admin"))

		Expect(err).To(MatchError(ContainSubstring("is not a ServiceAccount")))
	})
})
