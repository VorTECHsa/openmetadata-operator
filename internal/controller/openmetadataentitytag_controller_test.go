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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/finalizer"
	"github.com/VorTECHsa/openmetadata-operator/internal/handler"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

// stubEntityTagClient implements omclient.EntityTagClient with recorded calls.
type stubEntityTagClient struct {
	searchResp  []omclient.EntitySummary
	tagIDs      map[string]string
	addCalls    []stubBulkCall
	removeCalls []stubBulkCall
}

type stubBulkCall struct {
	tagID  string
	assets []omclient.AssetRef
}

func (s *stubEntityTagClient) SearchEntities(_ context.Context, _ string, _, _ []string) ([]omclient.EntitySummary, error) {
	return s.searchResp, nil
}

func (s *stubEntityTagClient) GetEntityByName(_ context.Context, _, fqn string) (string, error) {
	if id, ok := s.tagIDs[fqn]; ok {
		return id, nil
	}
	return "", &omclient.APIError{StatusCode: 404, Body: "not found"}
}

func (s *stubEntityTagClient) BulkAddTagToAssets(_ context.Context, tagID string, assets []omclient.AssetRef) error {
	s.addCalls = append(s.addCalls, stubBulkCall{tagID: tagID, assets: assets})
	return nil
}

func (s *stubEntityTagClient) BulkRemoveTagFromAssets(_ context.Context, tagID string, assets []omclient.AssetRef) error {
	s.removeCalls = append(s.removeCalls, stubBulkCall{tagID: tagID, assets: assets})
	return nil
}

func newEntityTagReconciler(stub *stubEntityTagClient) *OpenMetadataEntityTagReconciler {
	return &OpenMetadataEntityTagReconciler{
		Client: k8sClient,
		Handler: &handler.EntityTagHandler{
			Client:      k8sClient,
			NewOMClient: func(_, _ string) omclient.EntityTagClient { return stub },
		},
	}
}

var _ = Describe("OpenMetadataEntityTag Controller", func() {
	const (
		resourceName   = "trading-kafka-raw"
		namespace      = "default"
		connectionName = "om-connection-et"
	)

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}

	createPrereqs := func() {
		conn := &omv1alpha1.OpenMetadataConnection{
			ObjectMeta: metav1.ObjectMeta{Name: connectionName},
			Spec: omv1alpha1.OpenMetadataConnectionSpec{
				URL: "http://openmetadata:8585/api",
				AuthSecretRef: omv1alpha1.SecretReference{
					Name: "openmetadata-api-secret-et", Namespace: namespace, Key: "token",
				},
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "openmetadata-api-secret-et", Namespace: namespace},
			Data:       map[string][]byte{"token": []byte("test-jwt")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
	}

	createCR := func() *omv1alpha1.OpenMetadataEntityTag {
		return &omv1alpha1.OpenMetadataEntityTag{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec: omv1alpha1.OpenMetadataEntityTagSpec{
				Match: omv1alpha1.EntityMatch{
					EntityType: omv1alpha1.TaggableEntityTypeTopic,
					Includes:   []string{"trading-kafka.raw.*"},
				},
				Tag:                       omv1alpha1.TagRef{TagFQN: "Tier.Tier5"},
				OpenMetadataConnectionRef: connectionName,
			},
		}
	}

	cleanUp := func() {
		et := &omv1alpha1.OpenMetadataEntityTag{}
		if err := k8sClient.Get(ctx, namespacedName, et); err == nil {
			et.Finalizers = nil
			_ = k8sClient.Update(ctx, et)
			_ = k8sClient.Delete(ctx, et)
		}
		conn := &omv1alpha1.OpenMetadataConnection{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: connectionName}, conn); err == nil {
			_ = k8sClient.Delete(ctx, conn)
		}
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openmetadata-api-secret-et", Namespace: namespace}, secret); err == nil {
			_ = k8sClient.Delete(ctx, secret)
		}
	}

	BeforeEach(func() { createPrereqs() })
	AfterEach(func() { cleanUp() })

	It("returns empty result when CR does not exist", func() {
		stub := &stubEntityTagClient{}
		reconciler := newEntityTagReconciler(stub)

		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
	})

	It("adds finalizer on first reconciliation", func() {
		Expect(k8sClient.Create(ctx, createCR())).To(Succeed())
		stub := &stubEntityTagClient{}
		reconciler := newEntityTagReconciler(stub)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &omv1alpha1.OpenMetadataEntityTag{}
		Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
		Expect(updated.Finalizers).To(ContainElement(finalizer.Name))
	})

	It("applies tags to matched entities and records them in status", func() {
		Expect(k8sClient.Create(ctx, createCR())).To(Succeed())
		// Filtering is server-side via ES, so the stub returns the already-matched
		// set (what the OM /search/query endpoint would return for the include pattern).
		stub := &stubEntityTagClient{
			searchResp: []omclient.EntitySummary{
				{ID: "uuid-raw-events", FullyQualifiedName: "trading-kafka.raw.events"},
				{ID: "uuid-raw-orders", FullyQualifiedName: "trading-kafka.raw.orders"},
			},
			tagIDs: map[string]string{"Tier.Tier5": "tier5-uuid"},
		}
		reconciler := newEntityTagReconciler(stub)

		// First reconcile adds finalizer.
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// Second reconcile applies tags.
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		// Both raw.* topics should have been added to the bulk call.
		Expect(stub.addCalls).To(HaveLen(1))
		Expect(stub.addCalls[0].tagID).To(Equal("tier5-uuid"))
		Expect(stub.addCalls[0].assets).To(HaveLen(2))
		Expect(stub.removeCalls).To(BeEmpty())

		updated := &omv1alpha1.OpenMetadataEntityTag{}
		Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
		Expect(updated.Status.TagAssignments).To(HaveLen(2))
	})

	It("removes tags on deletion via finalizer", func() {
		Expect(k8sClient.Create(ctx, createCR())).To(Succeed())
		stub := &stubEntityTagClient{
			searchResp: []omclient.EntitySummary{
				{ID: "uuid-raw-events", FullyQualifiedName: "trading-kafka.raw.events"},
			},
			tagIDs: map[string]string{"Tier.Tier5": "tier5-uuid"},
		}
		reconciler := newEntityTagReconciler(stub)

		// Reconcile twice: first adds the finalizer, second populates status.TagAssignments.
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// Trigger deletion.
		et := &omv1alpha1.OpenMetadataEntityTag{}
		Expect(k8sClient.Get(ctx, namespacedName, et)).To(Succeed())
		Expect(k8sClient.Delete(ctx, et)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
		Expect(err).NotTo(HaveOccurred())

		// Bulk-remove was invoked for the previously-applied tag.
		Expect(stub.removeCalls).To(HaveLen(1))
		Expect(stub.removeCalls[0].tagID).To(Equal("tier5-uuid"))
		Expect(stub.removeCalls[0].assets).To(HaveLen(1))
		Expect(stub.removeCalls[0].assets[0].ID).To(Equal("uuid-raw-events"))
	})
})
