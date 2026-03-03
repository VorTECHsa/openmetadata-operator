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

// stubPipelineClient implements omclient.PipelineClient with fixed responses.
type stubPipelineClient struct {
	getResp        *omclient.PipelineResponse
	postDeployResp *omclient.PipelineResponse
	deployed       bool
	upsertResp     *omclient.PipelineResponse
	deployErr      error
	deleteErr      error
	entityID       string
}

func (s *stubPipelineClient) GetPipelineByFQN(_ context.Context, _ string) (*omclient.PipelineResponse, error) {
	if s.deployed && s.postDeployResp != nil {
		return s.postDeployResp, nil
	}
	if s.getResp == nil {
		return nil, &omclient.APIError{StatusCode: 404, Body: "not found"}
	}
	return s.getResp, nil
}

func (s *stubPipelineClient) UpsertPipeline(_ context.Context, _ omclient.PipelineRequest) (*omclient.PipelineResponse, error) {
	return s.upsertResp, nil
}

func (s *stubPipelineClient) DeployPipeline(_ context.Context, _ string) error {
	s.deployed = true
	return s.deployErr
}

func (s *stubPipelineClient) DeletePipeline(_ context.Context, _ string) error {
	return s.deleteErr
}

func (s *stubPipelineClient) GetEntityByName(_ context.Context, _, _ string) (string, error) {
	return s.entityID, nil
}

func newTestPipelineReconciler(stub omclient.PipelineClient) *IngestionPipelineReconciler {
	return &IngestionPipelineReconciler{
		Client: k8sClient,
		Handler: &handler.PipelineHandler{
			Client:      k8sClient,
			NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
		},
	}
}

var _ = Describe("IngestionPipeline Controller", func() {
	const (
		resourceName        = "test-metadata-pipeline"
		namespace           = "default"
		pipelineConnName    = "om-pipeline-connection"
		stubOMPipelineID    = "pipeline-uuid-12345"
		stubOMVersion       = 0.1
		stubOMVersionString = "0.1"
		stubServiceID       = "service-uuid-1"
	)

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}

	createPipelinePrerequisites := func() {
		conn := &omv1alpha1.OpenMetadataConnection{
			ObjectMeta: metav1.ObjectMeta{
				Name: pipelineConnName,
			},
			Spec: omv1alpha1.OpenMetadataConnectionSpec{
				URL: "http://openmetadata:8585/api",
				AuthSecretRef: omv1alpha1.SecretReference{
					Name:      "om-pipeline-api-secret",
					Namespace: namespace,
					Key:       "token",
				},
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		authSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "om-pipeline-api-secret",
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"token": []byte("test-jwt-token"),
			},
		}
		Expect(k8sClient.Create(ctx, authSecret)).To(Succeed())
	}

	createPipelineCR := func() *omv1alpha1.IngestionPipeline {
		return &omv1alpha1.IngestionPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: omv1alpha1.IngestionPipelineSpec{
				ForOpenMetadata: omv1alpha1.IngestionPipelineOMSpec{
					PipelineType: omv1alpha1.PipelineTypeMetadata,
					Service: omv1alpha1.EntityReference{
						FullyQualifiedName: "my-postgres-service",
						Type:               omv1alpha1.EntityTypeDatabaseService,
					},
					SourceConfig: mustRawExtension(map[string]any{
						"type":          "DatabaseMetadata",
						"includeTables": true,
					}),
					AirflowConfig: omv1alpha1.AirflowConfig{
						ScheduleInterval: "0 2 * * *",
					},
				},
				OpenMetadataConnectionRef: pipelineConnName,
			},
		}
	}

	cleanUpPipeline := func() {
		resource := &omv1alpha1.IngestionPipeline{}
		if err := k8sClient.Get(ctx, namespacedName, resource); err == nil {
			resource.Finalizers = nil
			_ = k8sClient.Update(ctx, resource)
			_ = k8sClient.Delete(ctx, resource)
		}
		conn := &omv1alpha1.OpenMetadataConnection{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: pipelineConnName}, conn); err == nil {
			_ = k8sClient.Delete(ctx, conn)
		}
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "om-pipeline-api-secret", Namespace: namespace}, secret); err == nil {
			_ = k8sClient.Delete(ctx, secret)
		}
	}

	pipelineStub := &stubPipelineClient{
		getResp: nil, // pipeline does not exist yet
		upsertResp: &omclient.PipelineResponse{
			ID:                 stubOMPipelineID,
			Name:               resourceName,
			FullyQualifiedName: "my-postgres-service." + resourceName,
			PipelineType:       "metadata",
			Deployed:           false,
			Version:            stubOMVersion,
		},
		postDeployResp: &omclient.PipelineResponse{
			ID:                 stubOMPipelineID,
			Name:               resourceName,
			FullyQualifiedName: "my-postgres-service." + resourceName,
			Version:            stubOMVersion + 0.1,
			Deployed:           true,
		},
		entityID: stubServiceID,
	}

	BeforeEach(func() {
		createPipelinePrerequisites()
	})

	AfterEach(func() {
		cleanUpPipeline()
	})

	It("should return empty result when CR does not exist", func() {
		reconciler := newTestPipelineReconciler(pipelineStub)

		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
	})

	It("should add a finalizer on first reconciliation", func() {
		Expect(k8sClient.Create(ctx, createPipelineCR())).To(Succeed())
		reconciler := newTestPipelineReconciler(pipelineStub)

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		updated := &omv1alpha1.IngestionPipeline{}
		Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
		Expect(updated.Finalizers).To(ContainElement(finalizer.Name))
	})

	It("should register and deploy the pipeline after finalizer is present", func() {
		Expect(k8sClient.Create(ctx, createPipelineCR())).To(Succeed())
		reconciler := newTestPipelineReconciler(pipelineStub)

		// First reconcile: adds finalizer
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// Second reconcile: delegates to handler, which upserts + deploys
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		updated := &omv1alpha1.IngestionPipeline{}
		Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
		Expect(updated.Status.OpenMetadataID).To(Equal(stubOMPipelineID))
		Expect(updated.Status.FullyQualifiedName).To(Equal("my-postgres-service." + resourceName))
		Expect(updated.Status.LastReconcileTime).NotTo(BeNil())
		Expect(updated.Status.RemoteVersion).To(Equal("0.2")) // post-deploy version
		Expect(updated.Status.Deployed).To(BeTrue())

		readyCond := condition.FindReady(updated.Status.Conditions)
		Expect(readyCond).NotTo(BeNil())
		Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(readyCond.Reason).To(Equal(omv1alpha1.ReasonCreated))
	})

	It("should clean up the pipeline and remove finalizer on deletion", func() {
		Expect(k8sClient.Create(ctx, createPipelineCR())).To(Succeed())
		reconciler := newTestPipelineReconciler(pipelineStub)

		// Reconcile twice to add finalizer and register pipeline
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// Delete the CR
		current := &omv1alpha1.IngestionPipeline{}
		Expect(k8sClient.Get(ctx, namespacedName, current)).To(Succeed())
		Expect(k8sClient.Delete(ctx, current)).To(Succeed())

		// Reconcile deletion
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// CR should be gone
		err = k8sClient.Get(ctx, namespacedName, &omv1alpha1.IngestionPipeline{})
		Expect(errors.IsNotFound(err)).To(BeTrue())
	})
})
