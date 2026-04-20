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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/condition"
	"github.com/VorTECHsa/openmetadata-operator/internal/finalizer"
	"github.com/VorTECHsa/openmetadata-operator/internal/handler"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

// stubClient implements omclient.ServiceClient with fixed responses.
type stubClient struct {
	getResp    *omclient.ServiceResponse
	upsertResp *omclient.ServiceResponse
	deleteErr  error
}

func (s *stubClient) GetServiceByName(_ context.Context, _, _ string) (*omclient.ServiceResponse, error) {
	if s.getResp == nil {
		return nil, &omclient.APIError{StatusCode: 404, Body: "not found"}
	}
	return s.getResp, nil
}

func (s *stubClient) UpsertService(_ context.Context, _ string, _ omclient.ServiceRequest) (*omclient.ServiceResponse, error) {
	return s.upsertResp, nil
}

func (s *stubClient) DeleteService(_ context.Context, _, _ string) error {
	return s.deleteErr
}

func (s *stubClient) GetEntityByName(_ context.Context, _, _ string) (string, error) {
	return "", &omclient.APIError{StatusCode: 404, Body: "not found"}
}

func newTestReconciler(stub omclient.ServiceClient) *OpenMetadataServiceReconciler {
	return &OpenMetadataServiceReconciler{
		Client: k8sClient,
		Handler: &handler.ServiceHandler{
			Client:      k8sClient,
			NewOMClient: func(_, _ string) omclient.ServiceClient { return stub },
		},
	}
}

var _ = Describe("OpenMetadataService Controller", func() {
	const (
		resourceName        = "test-postgres-service"
		namespace           = "default"
		connectionName      = "om-connection"
		stubOMEntityID      = "om-uuid-12345"
		stubOMVersion       = 0.1
		stubOMVersionString = "0.1"
	)

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}

	createTestPrerequisites := func() {
		conn := &omv1alpha1.OpenMetadataConnection{
			ObjectMeta: metav1.ObjectMeta{
				Name: connectionName,
			},
			Spec: omv1alpha1.OpenMetadataConnectionSpec{
				URL: "http://openmetadata:8585/api",
				AuthSecretRef: omv1alpha1.SecretReference{
					Name:      "openmetadata-api-secret",
					Namespace: namespace,
					Key:       "token",
				},
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		authSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openmetadata-api-secret",
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"token": []byte("test-jwt-token"),
			},
		}
		Expect(k8sClient.Create(ctx, authSecret)).To(Succeed())

		dbSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "analytics-db-output",
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"endpoint": []byte("db.example.com:5432"),
				"username": []byte("admin"),
				"password": []byte("s3cret"),
			},
		}
		Expect(k8sClient.Create(ctx, dbSecret)).To(Succeed())
	}

	createTestCR := func() *omv1alpha1.OpenMetadataService {
		return &omv1alpha1.OpenMetadataService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: omv1alpha1.OpenMetadataServiceSpec{
				ForOpenMetadata: omv1alpha1.ServiceOMSpec{
					ServiceType: omv1alpha1.ServiceTypePostgres,
					DisplayName: "Test Postgres Service",
					Description: "Test service for integration tests",
					Connection: omv1alpha1.ConnectionSpec{
						Config: mustRawExtension(map[string]any{
							"type": map[string]any{"value": "Postgres"},
							"hostPort": map[string]any{
								"valueFrom": map[string]any{
									"secretKeyRef": map[string]any{
										"name": "analytics-db-output",
										"key":  "endpoint",
									},
								},
							},
							"database": map[string]any{"value": "testdb"},
							"username": map[string]any{
								"valueFrom": map[string]any{
									"secretKeyRef": map[string]any{
										"name": "analytics-db-output",
										"key":  "username",
									},
								},
							},
							"authType": map[string]any{
								"password": map[string]any{
									"valueFrom": map[string]any{
										"secretKeyRef": map[string]any{
											"name": "analytics-db-output",
											"key":  "password",
										},
									},
								},
							},
						}),
					},
				},
				OpenMetadataConnectionRef: connectionName,
			},
		}
	}

	cleanUp := func() {
		resource := &omv1alpha1.OpenMetadataService{}
		if err := k8sClient.Get(ctx, namespacedName, resource); err == nil {
			resource.Finalizers = nil
			_ = k8sClient.Update(ctx, resource)
			_ = k8sClient.Delete(ctx, resource)
		}
		conn := &omv1alpha1.OpenMetadataConnection{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: connectionName}, conn); err == nil {
			_ = k8sClient.Delete(ctx, conn)
		}
		for _, name := range []string{"openmetadata-api-secret", "analytics-db-output"} {
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		}
	}

	stub := &stubClient{
		getResp: nil, // service does not exist yet
		upsertResp: &omclient.ServiceResponse{
			ID:                 stubOMEntityID,
			Name:               resourceName,
			FullyQualifiedName: resourceName,
			ServiceType:        "Postgres",
			Version:            stubOMVersion,
		},
	}

	BeforeEach(func() {
		createTestPrerequisites()
	})

	AfterEach(func() {
		cleanUp()
	})

	It("should return empty result when CR does not exist", func() {
		reconciler := newTestReconciler(stub)

		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
	})

	It("should add a finalizer on first reconciliation", func() {
		Expect(k8sClient.Create(ctx, createTestCR())).To(Succeed())
		reconciler := newTestReconciler(stub)

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		updated := &omv1alpha1.OpenMetadataService{}
		Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
		Expect(updated.Finalizers).To(ContainElement(finalizer.Name))
	})

	It("should register the service and set Ready condition after finalizer is present", func() {
		Expect(k8sClient.Create(ctx, createTestCR())).To(Succeed())
		reconciler := newTestReconciler(stub)

		// First reconcile: adds finalizer
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// Second reconcile: delegates to handler, which upserts
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		updated := &omv1alpha1.OpenMetadataService{}
		Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
		Expect(updated.Status.OpenMetadataID).To(Equal(stubOMEntityID))
		Expect(updated.Status.FullyQualifiedName).To(Equal(resourceName))
		Expect(updated.Status.LastReconcileTime).NotTo(BeNil())
		Expect(updated.Status.RemoteVersion).To(Equal(stubOMVersionString))

		readyCond := condition.FindReady(updated.Status.Conditions)
		Expect(readyCond).NotTo(BeNil())
		Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(readyCond.Reason).To(Equal(omv1alpha1.ReasonCreated))
	})

	It("should clean up the OM service and remove finalizer on deletion", func() {
		Expect(k8sClient.Create(ctx, createTestCR())).To(Succeed())
		reconciler := newTestReconciler(stub)

		// Reconcile twice to add finalizer and register service
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// Delete the CR
		current := &omv1alpha1.OpenMetadataService{}
		Expect(k8sClient.Get(ctx, namespacedName, current)).To(Succeed())
		Expect(k8sClient.Delete(ctx, current)).To(Succeed())

		// Reconcile deletion — should call OM delete and remove finalizer
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// CR should be gone
		err = k8sClient.Get(ctx, namespacedName, &omv1alpha1.OpenMetadataService{})
		Expect(errors.IsNotFound(err)).To(BeTrue())
	})
})

// mustRawExtension marshals a map to runtime.RawExtension. Panics on error.
func mustRawExtension(m map[string]any) runtime.RawExtension {
	raw, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return runtime.RawExtension{Raw: raw}
}
