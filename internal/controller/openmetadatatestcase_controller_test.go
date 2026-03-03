/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/condition"
	"github.com/VorTECHsa/openmetadata-operator/internal/finalizer"
	"github.com/VorTECHsa/openmetadata-operator/internal/handler"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

// stubTestCaseClient implements omclient.TestCaseClient with fixed responses.
type stubTestCaseClient struct {
	getResp    *omclient.TestCaseResponse
	upsertResp *omclient.TestCaseResponse
	deleteErr  error
}

func (s *stubTestCaseClient) GetTestCaseByFQN(_ context.Context, _ string) (*omclient.TestCaseResponse, error) {
	if s.getResp == nil {
		return nil, &omclient.APIError{StatusCode: 404, Body: "not found"}
	}
	return s.getResp, nil
}

func (s *stubTestCaseClient) UpsertTestCase(_ context.Context, _ omclient.TestCaseRequest) (*omclient.TestCaseResponse, error) {
	return s.upsertResp, nil
}

func (s *stubTestCaseClient) DeleteTestCase(_ context.Context, _ string) error {
	return s.deleteErr
}

func newTestTestCaseReconciler(stub omclient.TestCaseClient) *OpenMetadataTestCaseReconciler {
	return &OpenMetadataTestCaseReconciler{
		Client: k8sClient,
		Handler: &handler.TestCaseHandler{
			Client:      k8sClient,
			NewOMClient: func(_, _ string) omclient.TestCaseClient { return stub },
		},
	}
}

var _ = Describe("OpenMetadataTestCase Controller", func() {
	const (
		resourceName     = "test-orders-not-null"
		namespace        = "default"
		tcConnName       = "om-tc-connection"
		stubOMTestCaseID = "tc-uuid-12345"
		stubOMSuiteID    = "suite-uuid-12345"
		stubOMVersion    = 0.1
		stubOMVersionStr = "0.1"
	)

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}

	createTestCasePrerequisites := func() {
		conn := &omv1alpha1.OpenMetadataConnection{
			ObjectMeta: metav1.ObjectMeta{
				Name: tcConnName,
			},
			Spec: omv1alpha1.OpenMetadataConnectionSpec{
				URL: "http://openmetadata:8585/api",
				AuthSecretRef: omv1alpha1.SecretReference{
					Name:      "om-tc-api-secret",
					Namespace: namespace,
					Key:       "token",
				},
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		authSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "om-tc-api-secret",
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"token": []byte("test-jwt-token"),
			},
		}
		Expect(k8sClient.Create(ctx, authSecret)).To(Succeed())
	}

	createTestCaseCR := func() *omv1alpha1.OpenMetadataTestCase {
		return &omv1alpha1.OpenMetadataTestCase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: omv1alpha1.OpenMetadataTestCaseSpec{
				ForOpenMetadata: omv1alpha1.TestCaseOMSpec{
					TestDefinition: "columnValuesToBeNotNull",
					EntityLink:     "<#E::table::my-postgres.my_db.public.orders::columns::status>",
				},
				OpenMetadataConnectionRef: tcConnName,
			},
		}
	}

	cleanUpTestCase := func() {
		resource := &omv1alpha1.OpenMetadataTestCase{}
		if err := k8sClient.Get(ctx, namespacedName, resource); err == nil {
			resource.Finalizers = nil
			_ = k8sClient.Update(ctx, resource)
			_ = k8sClient.Delete(ctx, resource)
		}
		conn := &omv1alpha1.OpenMetadataConnection{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: tcConnName}, conn); err == nil {
			_ = k8sClient.Delete(ctx, conn)
		}
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "om-tc-api-secret", Namespace: namespace}, secret); err == nil {
			_ = k8sClient.Delete(ctx, secret)
		}
	}

	tcStub := &stubTestCaseClient{
		getResp: nil, // test case does not exist yet
		upsertResp: &omclient.TestCaseResponse{
			ID:                 stubOMTestCaseID,
			Name:               resourceName,
			FullyQualifiedName: "my-postgres.my_db.public.orders." + resourceName,
			TestSuite: struct {
				ID string `json:"id"`
			}{ID: stubOMSuiteID},
			Version: stubOMVersion,
		},
	}

	BeforeEach(func() {
		createTestCasePrerequisites()
	})

	AfterEach(func() {
		cleanUpTestCase()
	})

	It("should return empty result when CR does not exist", func() {
		reconciler := newTestTestCaseReconciler(tcStub)

		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
	})

	It("should add a finalizer on first reconciliation", func() {
		Expect(k8sClient.Create(ctx, createTestCaseCR())).To(Succeed())
		reconciler := newTestTestCaseReconciler(tcStub)

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		updated := &omv1alpha1.OpenMetadataTestCase{}
		Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
		Expect(updated.Finalizers).To(ContainElement(finalizer.Name))
	})

	It("should register the test case after finalizer is present", func() {
		Expect(k8sClient.Create(ctx, createTestCaseCR())).To(Succeed())
		reconciler := newTestTestCaseReconciler(tcStub)

		// First reconcile: adds finalizer
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// Second reconcile: delegates to handler, which upserts
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		updated := &omv1alpha1.OpenMetadataTestCase{}
		Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
		Expect(updated.Status.OpenMetadataID).To(Equal(stubOMTestCaseID))
		Expect(updated.Status.FullyQualifiedName).To(Equal("my-postgres.my_db.public.orders." + resourceName))
		Expect(updated.Status.TestSuiteID).To(Equal(stubOMSuiteID))
		Expect(updated.Status.LastReconcileTime).NotTo(BeNil())
		Expect(updated.Status.RemoteVersion).To(Equal(stubOMVersionStr))

		readyCond := condition.FindReady(updated.Status.Conditions)
		Expect(readyCond).NotTo(BeNil())
		Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(readyCond.Reason).To(Equal(omv1alpha1.ReasonCreated))
	})

	It("should clean up the test case and remove finalizer on deletion", func() {
		Expect(k8sClient.Create(ctx, createTestCaseCR())).To(Succeed())
		reconciler := newTestTestCaseReconciler(tcStub)

		// Reconcile twice to add finalizer and register test case
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// Delete the CR
		current := &omv1alpha1.OpenMetadataTestCase{}
		Expect(k8sClient.Get(ctx, namespacedName, current)).To(Succeed())
		Expect(k8sClient.Delete(ctx, current)).To(Succeed())

		// Reconcile deletion
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// CR should be gone
		err = k8sClient.Get(ctx, namespacedName, &omv1alpha1.OpenMetadataTestCase{})
		Expect(errors.IsNotFound(err)).To(BeTrue())
	})
})
