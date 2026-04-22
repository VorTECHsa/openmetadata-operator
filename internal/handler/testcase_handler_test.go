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
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/condition"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

// stubTestCaseClient implements omclient.TestCaseClient for unit tests.
type stubTestCaseClient struct {
	getResp    *omclient.TestCaseResponse
	getErr     error
	upsertResp *omclient.TestCaseResponse
	upsertErr  error
	deleteErr  error

	// entityIDs maps "typePath/fqn" to a UUID for GetEntityByName lookups.
	entityIDs map[string]string

	// capturedReq stores the last request passed to UpsertTestCase.
	capturedReq *omclient.TestCaseRequest
}

func (s *stubTestCaseClient) UpsertTestCase(_ context.Context, req omclient.TestCaseRequest) (*omclient.TestCaseResponse, error) {
	s.capturedReq = &req
	return s.upsertResp, s.upsertErr
}

func (s *stubTestCaseClient) GetTestCaseByFQN(_ context.Context, _ string) (*omclient.TestCaseResponse, error) {
	return s.getResp, s.getErr
}

func (s *stubTestCaseClient) DeleteTestCase(_ context.Context, _ string) error {
	return s.deleteErr
}

func (s *stubTestCaseClient) GetEntityByName(_ context.Context, path, fqn string) (string, error) {
	if id, ok := s.entityIDs[path+"/"+fqn]; ok {
		return id, nil
	}
	return "", &omclient.APIError{StatusCode: 404, Body: "not found"}
}

const (
	testTestCaseID  = "tc-uuid-1"
	testTestCaseFQN = "my-postgres.my_db.public.orders.orders-status-not-null"
	testSuiteID     = "suite-uuid-1"
)

func newTestCaseCR() *omv1alpha1.OpenMetadataTestCase {
	return &omv1alpha1.OpenMetadataTestCase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "orders-status-not-null",
			Namespace:  testNamespace,
			Generation: 1,
		},
		Spec: omv1alpha1.OpenMetadataTestCaseSpec{
			ForOpenMetadata: omv1alpha1.TestCaseOMSpec{
				TestDefinition: "columnValuesToBeNotNull",
				EntityLink:     "<#E::table::my-postgres.my_db.public.orders::columns::status>",
			},
			OpenMetadataConnectionRef: testConnectionName,
		},
	}
}

func TestTestCaseReconcileCreate(t *testing.T) {
	scheme := newTestScheme()
	tc := newTestCaseCR()
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tc, secret, newTestConnection()).
		WithStatusSubresource(tc).
		Build()

	stub := &stubTestCaseClient{
		getResp: nil,
		getErr:  &omclient.APIError{StatusCode: 404},
		upsertResp: &omclient.TestCaseResponse{
			ID:                 testTestCaseID,
			Name:               "orders-status-not-null",
			FullyQualifiedName: testTestCaseFQN,
			TestSuite: struct {
				ID string `json:"id"`
			}{ID: testSuiteID},
			Version: 0.1,
		},
	}

	h := &TestCaseHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.TestCaseClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}
	if tc.Status.OpenMetadataID != testTestCaseID {
		t.Errorf("expected OpenMetadataID %q, got %q", testTestCaseID, tc.Status.OpenMetadataID)
	}
	if tc.Status.TestSuiteID != testSuiteID {
		t.Errorf("expected TestSuiteID %q, got %q", testSuiteID, tc.Status.TestSuiteID)
	}

	readyCond := condition.FindReady(tc.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition to be set")
	}
	if readyCond.Reason != omv1alpha1.ReasonCreated {
		t.Errorf("expected reason 'Created', got %q", readyCond.Reason)
	}
}

func TestTestCaseReconcileInSync(t *testing.T) {
	scheme := newTestScheme()
	tc := newTestCaseCR()
	tc.Status.ObservedGeneration = tc.Generation
	tc.Status.RemoteVersion = testRemoteVersion
	tc.Status.OpenMetadataID = testTestCaseID
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tc, secret, newTestConnection()).
		WithStatusSubresource(tc).
		Build()

	stub := &stubTestCaseClient{
		getResp: &omclient.TestCaseResponse{
			ID:      testTestCaseID,
			Name:    "orders-status-not-null",
			Version: 0.1,
		},
	}

	h := &TestCaseHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.TestCaseClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}

	readyCond := condition.FindReady(tc.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonInSync {
		t.Errorf("expected reason 'InSync', got %q", readyCond.Reason)
	}
}

func TestTestCaseReconcileDetectsDrift(t *testing.T) {
	scheme := newTestScheme()
	tc := newTestCaseCR()
	tc.Status.ObservedGeneration = tc.Generation
	tc.Status.RemoteVersion = testRemoteVersion
	tc.Status.OpenMetadataID = testTestCaseID
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tc, secret, newTestConnection()).
		WithStatusSubresource(tc).
		Build()

	stub := &stubTestCaseClient{
		getResp: &omclient.TestCaseResponse{
			ID:      testTestCaseID,
			Name:    "orders-status-not-null",
			Version: 0.2, // different from stored 0.1
		},
		upsertResp: &omclient.TestCaseResponse{
			ID:                 testTestCaseID,
			Name:               "orders-status-not-null",
			FullyQualifiedName: testTestCaseFQN,
			TestSuite: struct {
				ID string `json:"id"`
			}{ID: testSuiteID},
			Version: 0.3,
		},
	}

	h := &TestCaseHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.TestCaseClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}

	readyCond := condition.FindReady(tc.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonDriftCorrected {
		t.Errorf("expected reason 'DriftCorrected', got %q", readyCond.Reason)
	}
}

func TestTestCaseReconcileUpsertFailure(t *testing.T) {
	scheme := newTestScheme()
	tc := newTestCaseCR()
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tc, secret, newTestConnection()).
		WithStatusSubresource(tc).
		Build()

	stub := &stubTestCaseClient{
		getResp:   nil,
		getErr:    &omclient.APIError{StatusCode: 404},
		upsertErr: fmt.Errorf("connection refused"),
	}

	h := &TestCaseHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.TestCaseClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), tc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no explicit requeue, got %v", result.RequeueAfter)
	}

	readyCond := condition.FindReady(tc.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonUpsertFailed {
		t.Errorf("expected reason %q, got %q", omv1alpha1.ReasonUpsertFailed, readyCond.Reason)
	}
}

func TestTestCaseReconcileWithParameterValues(t *testing.T) {
	scheme := newTestScheme()
	tc := newTestCaseCR()
	tc.Spec.ForOpenMetadata.ParameterValues = []omv1alpha1.TestCaseParameterValue{
		{Name: "minValue", Value: "0"},
		{Name: "maxValue", Value: "100000"},
	}
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tc, secret, newTestConnection()).
		WithStatusSubresource(tc).
		Build()

	stub := &stubTestCaseClient{
		getResp: nil,
		getErr:  &omclient.APIError{StatusCode: 404},
		upsertResp: &omclient.TestCaseResponse{
			ID:                 testTestCaseID,
			Name:               "orders-status-not-null",
			FullyQualifiedName: testTestCaseFQN,
			TestSuite: struct {
				ID string `json:"id"`
			}{ID: testSuiteID},
			Version: 0.1,
		},
	}

	h := &TestCaseHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.TestCaseClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}
	if tc.Status.OpenMetadataID != testTestCaseID {
		t.Errorf("expected OpenMetadataID %q, got %q", testTestCaseID, tc.Status.OpenMetadataID)
	}
}

func TestTestCaseHandleDeletionSuccess(t *testing.T) {
	scheme := newTestScheme()
	tc := newTestCaseCR()
	tc.Finalizers = []string{"openmetadata.vortexa.com/finalizer"}
	tc.Status.OpenMetadataID = testTestCaseID
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tc, secret, newTestConnection()).
		WithStatusSubresource(tc).
		Build()

	stub := &stubTestCaseClient{}

	h := &TestCaseHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.TestCaseClient { return stub },
	}

	result, err := h.HandleDeletion(context.Background(), tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}
	if len(tc.Finalizers) != 0 {
		t.Errorf("expected finalizer to be removed, got %v", tc.Finalizers)
	}
}

func TestTestCaseHandleDeletionWithoutFinalizer(t *testing.T) {
	tc := newTestCaseCR()

	h := &TestCaseHandler{}

	result, err := h.HandleDeletion(context.Background(), tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}
}

func TestTestCaseReconcileIncludesResolvedOwners(t *testing.T) {
	scheme := newTestScheme()
	tc := newTestCaseCR()
	tc.Spec.ForOpenMetadata.Owners = []omv1alpha1.EntityReference{
		{FullyQualifiedName: "platform-team", Type: omv1alpha1.EntityTypeTeam},
		{FullyQualifiedName: "alice", Type: omv1alpha1.EntityTypeUser},
	}
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tc, secret, newTestConnection()).
		WithStatusSubresource(tc).
		Build()

	stub := &stubTestCaseClient{
		getResp: nil,
		getErr:  &omclient.APIError{StatusCode: 404},
		entityIDs: map[string]string{
			"teams/platform-team": testTeamOwnerUUID,
			"users/alice":         testUserOwnerUUID,
		},
		upsertResp: &omclient.TestCaseResponse{
			ID:                 testTestCaseID,
			Name:               "orders-status-not-null",
			FullyQualifiedName: testTestCaseFQN,
			TestSuite: struct {
				ID string `json:"id"`
			}{ID: testSuiteID},
			Version: 0.1,
		},
	}

	h := &TestCaseHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.TestCaseClient { return stub },
	}

	if _, err := h.Reconcile(context.Background(), tc); err != nil {
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

func TestTestCaseReconcileUnsupportedOwnerTypeDoesNotRequeue(t *testing.T) {
	scheme := newTestScheme()
	tc := newTestCaseCR()
	tc.Spec.ForOpenMetadata.Owners = []omv1alpha1.EntityReference{
		{FullyQualifiedName: "some-db", Type: omv1alpha1.EntityTypeDatabaseService},
	}
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tc, secret, newTestConnection()).
		WithStatusSubresource(tc).
		Build()

	stub := &stubTestCaseClient{
		getResp: nil,
		getErr:  &omclient.APIError{StatusCode: 404},
	}

	h := &TestCaseHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.TestCaseClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), tc)
	if err != nil {
		t.Fatalf("expected nil error for permanent spec error, got %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue for permanent spec error, got %v", result.RequeueAfter)
	}

	readyCond := condition.FindReady(tc.Status.Conditions)
	if readyCond == nil || readyCond.Reason != omv1alpha1.ReasonOwnerResolutionFailed {
		t.Errorf("expected reason %q, got %+v", omv1alpha1.ReasonOwnerResolutionFailed, readyCond)
	}
}
