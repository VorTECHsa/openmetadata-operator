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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/condition"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

// stubPipelineClient implements omclient.PipelineClient for unit tests.
type stubPipelineClient struct {
	getResp        *omclient.PipelineResponse
	getErr         error
	postDeployResp *omclient.PipelineResponse
	deployed       bool
	upsertResp     *omclient.PipelineResponse
	upsertErr      error
	deployErr      error
	deleteErr      error

	// entityIDs maps "typePath/fqn" to a UUID for GetEntityByName lookups.
	entityIDs map[string]string

	// capturedReq stores the last request passed to UpsertPipeline.
	capturedReq *omclient.PipelineRequest
}

func (s *stubPipelineClient) UpsertPipeline(_ context.Context, req omclient.PipelineRequest) (*omclient.PipelineResponse, error) {
	s.capturedReq = &req
	return s.upsertResp, s.upsertErr
}

func (s *stubPipelineClient) GetPipelineByFQN(_ context.Context, _ string) (*omclient.PipelineResponse, error) {
	if s.deployed && s.postDeployResp != nil {
		return s.postDeployResp, nil
	}
	return s.getResp, s.getErr
}

func (s *stubPipelineClient) DeployPipeline(_ context.Context, _ string) error {
	s.deployed = true
	return s.deployErr
}

func (s *stubPipelineClient) DeletePipeline(_ context.Context, _ string) error {
	return s.deleteErr
}

func (s *stubPipelineClient) GetEntityByName(_ context.Context, path, fqn string) (string, error) {
	if id, ok := s.entityIDs[path+"/"+fqn]; ok {
		return id, nil
	}
	return "", &omclient.APIError{StatusCode: 404, Body: "not found"}
}

func newTestPipelineCR() *omv1alpha1.IngestionPipeline {
	return &omv1alpha1.IngestionPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-metadata",
			Namespace:  testNamespace,
			Generation: 1,
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
			OpenMetadataConnectionRef: testConnectionName,
		},
	}
}

const (
	testPipelineID      = "pipeline-uuid-1"
	testPipelineFQN     = "my-postgres-service.test-metadata"
	testServiceEntityID = "service-uuid-1"
)

func TestPipelineReconcileCreateAndDeploy(t *testing.T) {
	scheme := newTestScheme()
	pipeline := newTestPipelineCR()
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline, secret, newTestConnection()).
		WithStatusSubresource(pipeline).
		Build()

	stub := &stubPipelineClient{
		entityIDs: map[string]string{
			"services/databaseServices/my-postgres-service": testServiceEntityID,
		},
		getResp: nil, // pipeline does not exist
		getErr:  &omclient.APIError{StatusCode: 404},
		upsertResp: &omclient.PipelineResponse{
			ID:                 testPipelineID,
			Name:               "test-metadata",
			FullyQualifiedName: testPipelineFQN,
			PipelineType:       "metadata",
			Deployed:           false,
			Version:            0.1,
		},
		postDeployResp: &omclient.PipelineResponse{
			ID:                 testPipelineID,
			Name:               "test-metadata",
			FullyQualifiedName: testPipelineFQN,
			Version:            0.2,
			Deployed:           true,
		},
	}

	h := &PipelineHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), pipeline)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}
	if pipeline.Status.OpenMetadataID != testPipelineID {
		t.Errorf("expected OpenMetadataID %q, got %q", testPipelineID, pipeline.Status.OpenMetadataID)
	}
	if !pipeline.Status.Deployed {
		t.Error("expected Deployed=true")
	}

	readyCond := condition.FindReady(pipeline.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition to be set")
	}
	if readyCond.Reason != omv1alpha1.ReasonCreated {
		t.Errorf("expected reason 'Created', got %q", readyCond.Reason)
	}
}

func TestPipelineReconcileInSync(t *testing.T) {
	scheme := newTestScheme()
	pipeline := newTestPipelineCR()
	pipeline.Status.ObservedGeneration = pipeline.Generation
	pipeline.Status.RemoteVersion = testRemoteVersion
	pipeline.Status.OpenMetadataID = testPipelineID
	pipeline.Status.Deployed = true
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline, secret, newTestConnection()).
		WithStatusSubresource(pipeline).
		Build()

	stub := &stubPipelineClient{
		entityIDs: map[string]string{
			"services/databaseServices/my-postgres-service": testServiceEntityID,
		},
		getResp: &omclient.PipelineResponse{
			ID:       testPipelineID,
			Name:     "test-metadata",
			Version:  0.1,
			Deployed: true,
		},
	}

	h := &PipelineHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), pipeline)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}

	readyCond := condition.FindReady(pipeline.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonInSync {
		t.Errorf("expected reason 'InSync', got %q", readyCond.Reason)
	}
}

func TestPipelineReconcileDeployFailure(t *testing.T) {
	scheme := newTestScheme()
	pipeline := newTestPipelineCR()
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline, secret, newTestConnection()).
		WithStatusSubresource(pipeline).
		Build()

	stub := &stubPipelineClient{
		entityIDs: map[string]string{
			"services/databaseServices/my-postgres-service": testServiceEntityID,
		},
		getResp: nil,
		getErr:  &omclient.APIError{StatusCode: 404},
		upsertResp: &omclient.PipelineResponse{
			ID:                 testPipelineID,
			Name:               "test-metadata",
			FullyQualifiedName: testPipelineFQN,
			PipelineType:       "metadata",
			Version:            0.1,
		},
		deployErr: fmt.Errorf("airflow unavailable"),
	}

	h := &PipelineHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
	}

	_, err := h.Reconcile(context.Background(), pipeline)
	if err == nil {
		t.Fatal("expected error for deploy failure")
	}
	if pipeline.Status.Deployed {
		t.Error("expected Deployed=false after deploy failure")
	}

	readyCond := condition.FindReady(pipeline.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonDeployFailed {
		t.Errorf("expected reason 'DeployFailed', got %q", readyCond.Reason)
	}
}

func TestPipelineReconcileServiceNotFound(t *testing.T) {
	scheme := newTestScheme()
	pipeline := newTestPipelineCR()
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline, secret, newTestConnection()).
		WithStatusSubresource(pipeline).
		Build()

	stub := &stubPipelineClient{
		// entityIDs intentionally nil so service lookup returns 404
	}

	h := &PipelineHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
	}

	_, err := h.Reconcile(context.Background(), pipeline)
	if err == nil {
		t.Fatal("expected error for service not found")
	}

	readyCond := condition.FindReady(pipeline.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonServiceNotFound {
		t.Errorf("expected reason 'ServiceNotFound', got %q", readyCond.Reason)
	}
}

func TestPipelineReconcileDetectsDrift(t *testing.T) {
	scheme := newTestScheme()
	pipeline := newTestPipelineCR()
	pipeline.Status.ObservedGeneration = pipeline.Generation
	pipeline.Status.RemoteVersion = testRemoteVersion
	pipeline.Status.OpenMetadataID = testPipelineID
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline, secret, newTestConnection()).
		WithStatusSubresource(pipeline).
		Build()

	stub := &stubPipelineClient{
		entityIDs: map[string]string{
			"services/databaseServices/my-postgres-service": testServiceEntityID,
		},
		getResp: &omclient.PipelineResponse{
			ID:       testPipelineID,
			Name:     "test-metadata",
			Version:  0.2, // different from stored 0.1
			Deployed: true,
		},
		upsertResp: &omclient.PipelineResponse{
			ID:                 testPipelineID,
			Name:               "test-metadata",
			FullyQualifiedName: testPipelineFQN,
			PipelineType:       "metadata",
			Version:            0.3,
		},
		postDeployResp: &omclient.PipelineResponse{
			ID:      testPipelineID,
			Name:    "test-metadata",
			Version: 0.4,
		},
	}

	h := &PipelineHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), pipeline)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}

	readyCond := condition.FindReady(pipeline.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonDriftCorrected {
		t.Errorf("expected reason 'DriftCorrected', got %q", readyCond.Reason)
	}
}

func TestPipelineReconcileRedeploysUndeployed(t *testing.T) {
	scheme := newTestScheme()
	pipeline := newTestPipelineCR()
	pipeline.Status.ObservedGeneration = pipeline.Generation
	pipeline.Status.RemoteVersion = testRemoteVersion
	pipeline.Status.OpenMetadataID = testPipelineID
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline, secret, newTestConnection()).
		WithStatusSubresource(pipeline).
		Build()

	stub := &stubPipelineClient{
		entityIDs: map[string]string{
			"services/databaseServices/my-postgres-service": testServiceEntityID,
		},
		getResp: &omclient.PipelineResponse{
			ID:       testPipelineID,
			Name:     "test-metadata",
			Version:  0.1,
			Deployed: false, // not deployed
		},
		postDeployResp: &omclient.PipelineResponse{
			ID:      testPipelineID,
			Name:    "test-metadata",
			Version: 0.2,
		},
	}

	h := &PipelineHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), pipeline)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != requeueInterval {
		t.Errorf("expected RequeueAfter=%v, got %v", requeueInterval, result.RequeueAfter)
	}
	if !pipeline.Status.Deployed {
		t.Error("expected Deployed=true after re-deploy")
	}
}

func TestPipelineReconcileUpsertFailure(t *testing.T) {
	scheme := newTestScheme()
	pipeline := newTestPipelineCR()
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline, secret, newTestConnection()).
		WithStatusSubresource(pipeline).
		Build()

	stub := &stubPipelineClient{
		entityIDs: map[string]string{
			"services/databaseServices/my-postgres-service": testServiceEntityID,
		},
		getResp:   nil,
		getErr:    &omclient.APIError{StatusCode: 404},
		upsertErr: fmt.Errorf("connection refused"),
	}

	h := &PipelineHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), pipeline)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no explicit requeue, got %v", result.RequeueAfter)
	}

	readyCond := condition.FindReady(pipeline.Status.Conditions)
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Reason != omv1alpha1.ReasonUpsertFailed {
		t.Errorf("expected reason %q, got %q", omv1alpha1.ReasonUpsertFailed, readyCond.Reason)
	}
}

func TestPipelineHandleDeletionSuccess(t *testing.T) {
	scheme := newTestScheme()
	pipeline := newTestPipelineCR()
	pipeline.Finalizers = []string{"openmetadata.vortexa.com/finalizer"}
	pipeline.Status.OpenMetadataID = testPipelineID
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline, secret, newTestConnection()).
		WithStatusSubresource(pipeline).
		Build()

	stub := &stubPipelineClient{}

	h := &PipelineHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
	}

	result, err := h.HandleDeletion(context.Background(), pipeline)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}
	if len(pipeline.Finalizers) != 0 {
		t.Errorf("expected finalizer to be removed, got %v", pipeline.Finalizers)
	}
}

func TestPipelineHandleDeletionWithoutFinalizer(t *testing.T) {
	pipeline := newTestPipelineCR()

	h := &PipelineHandler{}

	result, err := h.HandleDeletion(context.Background(), pipeline)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}
}

func TestBuildAirflowConfig(t *testing.T) {
	startDate := metav1.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := omv1alpha1.AirflowConfig{
		ScheduleInterval: "0 2 * * *",
		StartDate:        &startDate,
		Concurrency:      2,
		Retries:          3,
		RetryDelay:       600,
		PausePipeline:    true,
		PipelineTimezone: "Europe/London",
		PipelineCatchup:  true,
		MaxActiveRuns:    5,
	}

	result := buildAirflowConfig(cfg)

	if result["scheduleInterval"] != "0 2 * * *" {
		t.Errorf("expected scheduleInterval '0 2 * * *', got %v", result["scheduleInterval"])
	}
	if result["concurrency"] != 2 {
		t.Errorf("expected concurrency 2, got %v", result["concurrency"])
	}
	if result["retries"] != 3 {
		t.Errorf("expected retries 3, got %v", result["retries"])
	}
	if result["pipelineTimezone"] != "Europe/London" {
		t.Errorf("expected pipelineTimezone 'Europe/London', got %v", result["pipelineTimezone"])
	}
	if result["maxActiveRuns"] != 5 {
		t.Errorf("expected maxActiveRuns 5, got %v", result["maxActiveRuns"])
	}
	if _, ok := result["startDate"]; !ok {
		t.Error("expected startDate to be set")
	}
}

func TestDecidePipelineAction(t *testing.T) {
	tests := []struct {
		name     string
		pipeline *omv1alpha1.IngestionPipeline
		existing *omclient.PipelineResponse
		want     pipelineAction
	}{
		{
			name:     "nil existing means create",
			pipeline: &omv1alpha1.IngestionPipeline{},
			existing: nil,
			want:     actionCreate,
		},
		{
			name: "generation mismatch means update",
			pipeline: &omv1alpha1.IngestionPipeline{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status:     omv1alpha1.IngestionPipelineStatus{ObservedGeneration: 1},
			},
			existing: &omclient.PipelineResponse{Version: 0.1, Deployed: true},
			want:     actionUpdate,
		},
		{
			name: "remote version mismatch means drift",
			pipeline: &omv1alpha1.IngestionPipeline{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     omv1alpha1.IngestionPipelineStatus{ObservedGeneration: 1, RemoteVersion: "0.1"},
			},
			existing: &omclient.PipelineResponse{Version: 0.2, Deployed: true},
			want:     actionCorrectDrift,
		},
		{
			name: "not deployed means redeploy",
			pipeline: &omv1alpha1.IngestionPipeline{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     omv1alpha1.IngestionPipelineStatus{ObservedGeneration: 1, RemoteVersion: "0.1"},
			},
			existing: &omclient.PipelineResponse{Version: 0.1, Deployed: false},
			want:     actionRedeploy,
		},
		{
			name: "everything matches means in sync",
			pipeline: &omv1alpha1.IngestionPipeline{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     omv1alpha1.IngestionPipelineStatus{ObservedGeneration: 1, RemoteVersion: "0.1"},
			},
			existing: &omclient.PipelineResponse{Version: 0.1, Deployed: true},
			want:     actionInSync,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decidePipelineAction(tt.pipeline, tt.existing)
			if got != tt.want {
				t.Errorf("decidePipelineAction() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildAirflowConfigEmpty(t *testing.T) {
	cfg := omv1alpha1.AirflowConfig{}
	result := buildAirflowConfig(cfg)

	if len(result) != 0 {
		t.Errorf("expected empty map for zero-value config, got %v", result)
	}
}

func TestPipelineReconcileIncludesResolvedOwners(t *testing.T) {
	scheme := newTestScheme()
	pipeline := newTestPipelineCR()
	pipeline.Spec.ForOpenMetadata.Owners = []omv1alpha1.EntityReference{
		{FullyQualifiedName: "platform-team", Type: omv1alpha1.EntityTypeTeam},
		{FullyQualifiedName: "alice", Type: omv1alpha1.EntityTypeUser},
	}
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline, secret, newTestConnection()).
		WithStatusSubresource(pipeline).
		Build()

	stub := &stubPipelineClient{
		entityIDs: map[string]string{
			"services/databaseServices/my-postgres-service": testServiceEntityID,
			"teams/platform-team":                           testTeamOwnerUUID,
			"users/alice":                                   testUserOwnerUUID,
		},
		getResp: nil,
		getErr:  &omclient.APIError{StatusCode: 404},
		upsertResp: &omclient.PipelineResponse{
			ID:                 testPipelineID,
			Name:               "test-metadata",
			FullyQualifiedName: testPipelineFQN,
			PipelineType:       "metadata",
			Deployed:           false,
			Version:            0.1,
		},
		postDeployResp: &omclient.PipelineResponse{
			ID: testPipelineID, Version: 0.2, Deployed: true,
		},
	}

	h := &PipelineHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
	}

	if _, err := h.Reconcile(context.Background(), pipeline); err != nil {
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

func TestPipelineReconcileUnsupportedOwnerTypeDoesNotRequeue(t *testing.T) {
	scheme := newTestScheme()
	pipeline := newTestPipelineCR()
	pipeline.Spec.ForOpenMetadata.Owners = []omv1alpha1.EntityReference{
		{FullyQualifiedName: "some-db", Type: omv1alpha1.EntityTypeDatabaseService},
	}
	secret := newAuthSecret()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline, secret, newTestConnection()).
		WithStatusSubresource(pipeline).
		Build()

	stub := &stubPipelineClient{
		entityIDs: map[string]string{
			"services/databaseServices/my-postgres-service": testServiceEntityID,
		},
		getResp: nil,
		getErr:  &omclient.APIError{StatusCode: 404},
	}

	h := &PipelineHandler{
		Client:      c,
		NewOMClient: func(_, _ string) omclient.PipelineClient { return stub },
	}

	result, err := h.Reconcile(context.Background(), pipeline)
	if err != nil {
		t.Fatalf("expected nil error for permanent spec error, got %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue for permanent spec error, got %v", result.RequeueAfter)
	}

	readyCond := condition.FindReady(pipeline.Status.Conditions)
	if readyCond == nil || readyCond.Reason != omv1alpha1.ReasonOwnerResolutionFailed {
		t.Errorf("expected reason %q, got %+v", omv1alpha1.ReasonOwnerResolutionFailed, readyCond)
	}
}
