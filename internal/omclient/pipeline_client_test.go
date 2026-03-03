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

package omclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpsertPipeline(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/v1/services/ingestionPipelines" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req PipelineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if req.Name != "my-metadata-pipeline" {
			t.Errorf("expected name 'my-metadata-pipeline', got %q", req.Name)
		}
		if req.PipelineType != "metadata" {
			t.Errorf("expected pipelineType 'metadata', got %q", req.PipelineType)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(PipelineResponse{
			ID:                 "pipeline-uuid-1",
			Name:               "my-metadata-pipeline",
			FullyQualifiedName: "my-service.my-metadata-pipeline",
			PipelineType:       "metadata",
			Deployed:           false,
			Version:            0.1,
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := NewPipelineClient(server.URL, "test-token")
	resp, err := client.UpsertPipeline(context.Background(), PipelineRequest{
		Name:          "my-metadata-pipeline",
		PipelineType:  "metadata",
		SourceConfig:  map[string]any{"type": "DatabaseMetadata"},
		AirflowConfig: map[string]any{"scheduleInterval": "0 2 * * *"},
		Service:       map[string]any{"id": "svc-uuid", "type": "databaseService"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "pipeline-uuid-1" {
		t.Errorf("expected ID 'pipeline-uuid-1', got %q", resp.ID)
	}
	if resp.FullyQualifiedName != "my-service.my-metadata-pipeline" {
		t.Errorf("expected FQN 'my-service.my-metadata-pipeline', got %q", resp.FullyQualifiedName)
	}
}

func TestUpsertPipelineError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":400,"message":"invalid payload"}`))
	}))
	defer server.Close()

	client := NewPipelineClient(server.URL, "test-token")
	_, err := client.UpsertPipeline(context.Background(), PipelineRequest{Name: "bad"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetPipelineByFQN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/services/ingestionPipelines/name/my-service.my-pipeline" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(PipelineResponse{
			ID:                 "pipeline-uuid-1",
			Name:               "my-pipeline",
			FullyQualifiedName: "my-service.my-pipeline",
			PipelineType:       "metadata",
			Deployed:           true,
			Version:            0.2,
		})
	}))
	defer server.Close()

	client := NewPipelineClient(server.URL, "test-token")
	resp, err := client.GetPipelineByFQN(context.Background(), "my-service.my-pipeline")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.ID != "pipeline-uuid-1" {
		t.Errorf("expected ID 'pipeline-uuid-1', got %q", resp.ID)
	}
	if !resp.Deployed {
		t.Error("expected Deployed=true")
	}
}

func TestGetPipelineByFQNNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
	}))
	defer server.Close()

	client := NewPipelineClient(server.URL, "test-token")
	resp, err := client.GetPipelineByFQN(context.Background(), "nonexistent")

	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("expected IsNotFound=true, got false for error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for 404, got %+v", resp)
	}
}

func TestDeployPipeline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/services/ingestionPipelines/deploy/pipeline-uuid-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPipelineClient(server.URL, "test-token")
	err := client.DeployPipeline(context.Background(), "pipeline-uuid-1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployPipelineError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`Airflow unavailable`))
	}))
	defer server.Close()

	client := NewPipelineClient(server.URL, "test-token")
	err := client.DeployPipeline(context.Background(), "pipeline-uuid-1")

	if err == nil {
		t.Fatal("expected error for 503, got nil")
	}
}

func TestDeletePipeline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/v1/services/ingestionPipelines/pipeline-uuid-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if !r.URL.Query().Has("hardDelete") {
			t.Error("expected hardDelete query param")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPipelineClient(server.URL, "test-token")
	err := client.DeletePipeline(context.Background(), "pipeline-uuid-1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeletePipelineNotFoundIsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewPipelineClient(server.URL, "test-token")
	err := client.DeletePipeline(context.Background(), "gone-id")

	if err != nil {
		t.Fatalf("expected 404 to be treated as success, got error: %v", err)
	}
}

func TestGetEntityByName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/services/databaseServices/name/my-postgres" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "svc-uuid-123"})
	}))
	defer server.Close()

	client := NewPipelineClient(server.URL, "test-token")
	id, err := client.GetEntityByName(context.Background(), "services/databaseServices", "my-postgres")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "svc-uuid-123" {
		t.Errorf("expected ID 'svc-uuid-123', got %q", id)
	}
}

func TestGetEntityByNameNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
	}))
	defer server.Close()

	client := NewPipelineClient(server.URL, "test-token")
	_, err := client.GetEntityByName(context.Background(), "services/databaseServices", "nonexistent")

	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("expected IsNotFound=true, got false for error: %v", err)
	}
}
