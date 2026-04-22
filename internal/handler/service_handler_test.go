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

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/condition"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

const (
	testNamespace      = "default"
	testConnectionName = "om-connection"
	testOpenMetadataID = "uuid-1"
	testRemoteVersion  = "0.1"
	testTeamOwnerUUID  = "team-uuid"
	testUserOwnerUUID  = "user-uuid"
)

// stubClient implements omclient.ServiceClient for unit tests.
type stubClient struct {
	getResp    *omclient.ServiceResponse
	getErr     error
	upsertResp *omclient.ServiceResponse
	upsertErr  error
	deleteErr  error

	// entityIDs maps "typePath/fqn" to a UUID for GetEntityByName lookups.
	entityIDs map[string]string

	// capturedReq stores the last request passed to UpsertService.
	capturedReq *omclient.ServiceRequest
}

func (s *stubClient) UpsertService(_ context.Context, _ string, req omclient.ServiceRequest) (*omclient.ServiceResponse, error) {
	s.capturedReq = &req
	return s.upsertResp, s.upsertErr
}

func (s *stubClient) GetServiceByName(_ context.Context, _, _ string) (*omclient.ServiceResponse, error) {
	return s.getResp, s.getErr
}

func (s *stubClient) DeleteService(_ context.Context, _, _ string) error {
	return s.deleteErr
}

func (s *stubClient) GetEntityByName(_ context.Context, path, fqn string) (string, error) {
	if id, ok := s.entityIDs[path+"/"+fqn]; ok {
		return id, nil
	}
	return "", &omclient.APIError{StatusCode: 404, Body: "not found"}
}

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = omv1alpha1.AddToScheme(s)
	return s
}

func mustRawExtension(m map[string]any) runtime.RawExtension {
	raw, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return runtime.RawExtension{Raw: raw}
}

func newTestServiceCR(name string) *omv1alpha1.OpenMetadataService {
	return &omv1alpha1.OpenMetadataService{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  testNamespace,
			Generation: 1,
		},
		Spec: omv1alpha1.OpenMetadataServiceSpec{
			ForOpenMetadata: omv1alpha1.ServiceOMSpec{
				ServiceType: omv1alpha1.ServiceTypePostgres,
				DisplayName: "Test Postgres",
				Connection: omv1alpha1.ConnectionSpec{
					Config: mustRawExtension(map[string]any{
						"type":     map[string]any{"value": "Postgres"},
						"hostPort": map[string]any{"value": "db.example.com:5432"},
						"database": map[string]any{"value": "testdb"},
					}),
				},
			},
			OpenMetadataConnectionRef: testConnectionName,
		},
	}
}

func newTestConnection() *omv1alpha1.OpenMetadataConnection {
	return &omv1alpha1.OpenMetadataConnection{
		ObjectMeta: metav1.ObjectMeta{
			Name: testConnectionName,
		},
		Spec: omv1alpha1.OpenMetadataConnectionSpec{
			URL: "http://openmetadata:8585/api",
			AuthSecretRef: omv1alpha1.SecretReference{
				Name:      "om-secret",
				Namespace: testNamespace,
				Key:       "token",
			},
		},
	}
}

func newAuthSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "om-secret", Namespace: testNamespace},
		Data:       map[string][]byte{"token": []byte("test-jwt")},
	}
}

func TestReconcileCreatesNewService(t *testing.T) {
	scheme := newTestScheme()
	svc := newTestServiceCR("test-pg")
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, secret, newTestConnection()).
		WithStatusSubresource(svc).
		Build()

	stub := &stubClient{
		getResp: nil, // service does not exist
		getErr:  &omclient.APIError{StatusCode: 404},
		upsertResp: &omclient.ServiceResponse{
			ID:                 testOpenMetadataID,
			Name:               "test-pg",
			FullyQualifiedName: "test-pg",
			ServiceType:        "Postgres",
			Version:            0.1,
		},
	}

	h := &ServiceHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.ServiceClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}
	if svc.Status.OpenMetadataID != testOpenMetadataID {
		t.Errorf("expected OpenMetadataID %q, got %q", testOpenMetadataID, svc.Status.OpenMetadataID)
	}

	readyCond := condition.FindReady(svc.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition to be set")
	}
	if readyCond.Reason != omv1alpha1.ReasonCreated {
		t.Errorf("expected reason 'Created', got %q", readyCond.Reason)
	}
}

func TestReconcileSkipsWhenInSync(t *testing.T) {
	scheme := newTestScheme()
	svc := newTestServiceCR("test-pg")
	svc.Status.ObservedGeneration = svc.Generation
	svc.Status.RemoteVersion = testRemoteVersion
	svc.Status.OpenMetadataID = testOpenMetadataID
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, secret, newTestConnection()).
		WithStatusSubresource(svc).
		Build()

	stub := &stubClient{
		getResp: &omclient.ServiceResponse{
			ID:      testOpenMetadataID,
			Name:    "test-pg",
			Version: 0.1,
		},
	}

	h := &ServiceHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.ServiceClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}

	readyCond := condition.FindReady(svc.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonInSync {
		t.Errorf("expected reason 'InSync', got %q", readyCond.Reason)
	}
}

func TestReconcileDetectsDrift(t *testing.T) {
	scheme := newTestScheme()
	svc := newTestServiceCR("test-pg")
	svc.Status.ObservedGeneration = svc.Generation
	svc.Status.RemoteVersion = testRemoteVersion
	svc.Status.OpenMetadataID = testOpenMetadataID
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, secret, newTestConnection()).
		WithStatusSubresource(svc).
		Build()

	stub := &stubClient{
		getResp: &omclient.ServiceResponse{
			ID:      testOpenMetadataID,
			Name:    "test-pg",
			Version: 0.2, // different from stored 0.1
		},
		upsertResp: &omclient.ServiceResponse{
			ID:                 testOpenMetadataID,
			Name:               "test-pg",
			FullyQualifiedName: "test-pg",
			ServiceType:        "Postgres",
			Version:            0.3,
		},
	}

	h := &ServiceHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.ServiceClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}

	readyCond := condition.FindReady(svc.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonDriftCorrected {
		t.Errorf("expected reason 'DriftCorrected', got %q", readyCond.Reason)
	}
}

func TestReconcileUnsupportedServiceType(t *testing.T) {
	scheme := newTestScheme()
	svc := newTestServiceCR("test-bad")
	svc.Spec.ForOpenMetadata.ServiceType = omv1alpha1.ServiceType("UnsupportedDB")
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, secret, newTestConnection()).
		WithStatusSubresource(svc).
		Build()

	h := &ServiceHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.ServiceClient { return &stubClient{} },
	}

	result, err := h.Reconcile(context.Background(), svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue for permanent error, got %v", result.RequeueAfter)
	}

	readyCond := condition.FindReady(svc.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Status != metav1.ConditionFalse {
		t.Errorf("expected False, got %s", readyCond.Status)
	}
	if readyCond.Reason != omv1alpha1.ReasonUnsupportedServiceType {
		t.Errorf("expected reason 'UnsupportedServiceType', got %q", readyCond.Reason)
	}
}

func TestReconcileUpdatesOnSpecChange(t *testing.T) {
	scheme := newTestScheme()
	svc := newTestServiceCR("test-pg")
	// Simulate a previous successful reconciliation.
	svc.Status.ObservedGeneration = 1
	svc.Status.RemoteVersion = testRemoteVersion
	svc.Status.OpenMetadataID = testOpenMetadataID
	// Bump generation to simulate a spec change.
	svc.Generation = 2
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, secret, newTestConnection()).
		WithStatusSubresource(svc).
		Build()

	stub := &stubClient{
		getResp: &omclient.ServiceResponse{
			ID:      testOpenMetadataID,
			Name:    "test-pg",
			Version: 0.1,
		},
		upsertResp: &omclient.ServiceResponse{
			ID:                 testOpenMetadataID,
			Name:               "test-pg",
			FullyQualifiedName: "test-pg",
			ServiceType:        "Postgres",
			Version:            0.2,
		},
	}

	h := &ServiceHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.ServiceClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}

	readyCond := condition.FindReady(svc.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonUpdated {
		t.Errorf("expected reason %q, got %q", omv1alpha1.ReasonUpdated, readyCond.Reason)
	}
	if svc.Status.ObservedGeneration != 2 {
		t.Errorf("expected ObservedGeneration=2, got %d", svc.Status.ObservedGeneration)
	}
}

func TestReconcileUpsertFailure(t *testing.T) {
	scheme := newTestScheme()
	svc := newTestServiceCR("test-pg")
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, secret, newTestConnection()).
		WithStatusSubresource(svc).
		Build()

	stub := &stubClient{
		getResp:   nil, // service does not exist
		getErr:    &omclient.APIError{StatusCode: 404},
		upsertErr: fmt.Errorf("connection refused"),
	}

	h := &ServiceHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.ServiceClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), svc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no explicit requeue, got %v", result.RequeueAfter)
	}

	readyCond := condition.FindReady(svc.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Status != metav1.ConditionFalse {
		t.Errorf("expected ConditionFalse, got %s", readyCond.Status)
	}
	if readyCond.Reason != omv1alpha1.ReasonUpsertFailed {
		t.Errorf("expected reason %q, got %q", omv1alpha1.ReasonUpsertFailed, readyCond.Reason)
	}
}

func TestHandleDeletionSuccess(t *testing.T) {
	scheme := newTestScheme()
	svc := newTestServiceCR("test-pg")
	svc.Finalizers = []string{"openmetadata.vortexa.com/finalizer"}
	svc.Status.OpenMetadataID = testOpenMetadataID
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, secret, newTestConnection()).
		WithStatusSubresource(svc).
		Build()

	stub := &stubClient{}

	h := &ServiceHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.ServiceClient { return stub },
	}

	result, err := h.HandleDeletion(context.Background(), svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}
	if len(svc.Finalizers) != 0 {
		t.Errorf("expected finalizer to be removed, got %v", svc.Finalizers)
	}
}

func TestHandleDeletionWithoutFinalizer(t *testing.T) {
	svc := newTestServiceCR("test-pg")
	// No finalizer set.

	h := &ServiceHandler{}

	result, err := h.HandleDeletion(context.Background(), svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}
}

func TestReconcileIncludesResolvedOwners(t *testing.T) {
	scheme := newTestScheme()
	svc := newTestServiceCR("test-pg")
	svc.Spec.ForOpenMetadata.Owners = []omv1alpha1.EntityReference{
		{FullyQualifiedName: "platform-team", Type: omv1alpha1.EntityTypeTeam},
		{FullyQualifiedName: "alice", Type: omv1alpha1.EntityTypeUser},
	}
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, secret, newTestConnection()).
		WithStatusSubresource(svc).
		Build()

	stub := &stubClient{
		getResp: nil,
		getErr:  &omclient.APIError{StatusCode: 404},
		entityIDs: map[string]string{
			"teams/platform-team": testTeamOwnerUUID,
			"users/alice":         testUserOwnerUUID,
		},
		upsertResp: &omclient.ServiceResponse{
			ID:                 testOpenMetadataID,
			Name:               "test-pg",
			FullyQualifiedName: "test-pg",
			ServiceType:        "Postgres",
			Version:            0.1,
		},
	}

	h := &ServiceHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.ServiceClient { return stub },
	}

	if _, err := h.Reconcile(context.Background(), svc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.capturedReq == nil {
		t.Fatal("expected upsert request to be captured")
	}
	if len(stub.capturedReq.Owners) != 2 {
		t.Fatalf("expected 2 owners, got %d", len(stub.capturedReq.Owners))
	}
	if stub.capturedReq.Owners[0].ID != testTeamOwnerUUID || stub.capturedReq.Owners[0].Type != string(omv1alpha1.EntityTypeTeam) {
		t.Errorf("unexpected first owner: %+v", stub.capturedReq.Owners[0])
	}
	if stub.capturedReq.Owners[1].ID != testUserOwnerUUID || stub.capturedReq.Owners[1].Type != string(omv1alpha1.EntityTypeUser) {
		t.Errorf("unexpected second owner: %+v", stub.capturedReq.Owners[1])
	}
}

func TestReconcileOwnerNotFoundRequeues(t *testing.T) {
	scheme := newTestScheme()
	svc := newTestServiceCR("test-pg")
	svc.Spec.ForOpenMetadata.Owners = []omv1alpha1.EntityReference{
		{FullyQualifiedName: "ghost", Type: omv1alpha1.EntityTypeUser},
	}
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, secret, newTestConnection()).
		WithStatusSubresource(svc).
		Build()

	stub := &stubClient{
		getResp:   nil,
		getErr:    &omclient.APIError{StatusCode: 404},
		entityIDs: map[string]string{},
	}

	h := &ServiceHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.ServiceClient { return stub },
	}

	if _, err := h.Reconcile(context.Background(), svc); err == nil {
		t.Fatal("expected error to trigger requeue when owner not found")
	}

	readyCond := condition.FindReady(svc.Status.Conditions)
	if readyCond == nil || readyCond.Reason != omv1alpha1.ReasonOwnerResolutionFailed {
		t.Errorf("expected reason %q, got %+v", omv1alpha1.ReasonOwnerResolutionFailed, readyCond)
	}
}

func TestReconcileUnsupportedOwnerTypeDoesNotRequeue(t *testing.T) {
	scheme := newTestScheme()
	svc := newTestServiceCR("test-pg")
	svc.Spec.ForOpenMetadata.Owners = []omv1alpha1.EntityReference{
		{FullyQualifiedName: "some-db", Type: omv1alpha1.EntityTypeDatabaseService},
	}
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, secret, newTestConnection()).
		WithStatusSubresource(svc).
		Build()

	stub := &stubClient{
		getResp: nil,
		getErr:  &omclient.APIError{StatusCode: 404},
	}

	h := &ServiceHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.ServiceClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), svc)
	if err != nil {
		t.Fatalf("expected nil error for permanent spec error, got %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue for permanent spec error, got %v", result.RequeueAfter)
	}

	readyCond := condition.FindReady(svc.Status.Conditions)
	if readyCond == nil || readyCond.Reason != omv1alpha1.ReasonOwnerResolutionFailed {
		t.Errorf("expected reason %q, got %+v", omv1alpha1.ReasonOwnerResolutionFailed, readyCond)
	}
}

func TestResolveAuthTokenMissing(t *testing.T) {
	scheme := newTestScheme()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	_, err := resolveAuthToken(context.Background(), c, omv1alpha1.SecretReference{
		Name: "nonexistent", Namespace: testNamespace, Key: "token",
	})
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestResolveAuthTokenMissingKey(t *testing.T) {
	scheme := newTestScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "om-secret", Namespace: testNamespace},
		Data:       map[string][]byte{"other": []byte("val")},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	_, err := resolveAuthToken(context.Background(), c, omv1alpha1.SecretReference{
		Name: "om-secret", Namespace: testNamespace, Key: "token",
	})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}
