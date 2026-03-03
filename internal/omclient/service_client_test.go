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

func TestUpsertService(t *testing.T) {
	// Mock HTTP server that simulates the OpenMetadata PUT endpoint.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/v1/services/databaseServices" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		var req ServiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if req.Name != "my-postgres" {
			t.Errorf("expected name 'my-postgres', got %q", req.Name)
		}
		if req.ServiceType != "Postgres" {
			t.Errorf("expected serviceType 'Postgres', got %q", req.ServiceType)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ServiceResponse{
			ID:                 "abc-123",
			Name:               "my-postgres",
			FullyQualifiedName: "my-postgres",
			ServiceType:        "Postgres",
			Version:            0.1,
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := NewServiceClient(server.URL, "test-token")
	resp, err := client.UpsertService(context.Background(), "databaseServices", ServiceRequest{
		Name:        "my-postgres",
		ServiceType: "Postgres",
		DisplayName: "My Postgres",
		Connection:  map[string]any{"config": map[string]any{"type": "Postgres"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "abc-123" {
		t.Errorf("expected ID 'abc-123', got %q", resp.ID)
	}
	if resp.FullyQualifiedName != "my-postgres" {
		t.Errorf("expected FQN 'my-postgres', got %q", resp.FullyQualifiedName)
	}
}

func TestUpsertServiceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":400,"message":"invalid payload"}`))
	}))
	defer server.Close()

	client := NewServiceClient(server.URL, "test-token")
	_, err := client.UpsertService(context.Background(), "databaseServices", ServiceRequest{
		Name: "bad-service",
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetServiceByName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/services/databaseServices/name/my-postgres" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ServiceResponse{
			ID:                 "abc-123",
			Name:               "my-postgres",
			FullyQualifiedName: "my-postgres",
			ServiceType:        "Postgres",
		})
	}))
	defer server.Close()

	client := NewServiceClient(server.URL, "test-token")
	resp, err := client.GetServiceByName(context.Background(), "databaseServices", "my-postgres")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.ID != "abc-123" {
		t.Errorf("expected ID 'abc-123', got %q", resp.ID)
	}
}

func TestGetServiceByNameNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
	}))
	defer server.Close()

	client := NewServiceClient(server.URL, "test-token")
	resp, err := client.GetServiceByName(context.Background(), "databaseServices", "nonexistent")

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

func TestDeleteService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/v1/services/databaseServices/abc-123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("recursive") != "true" {
			t.Error("expected recursive=true query param")
		}
		if r.URL.Query().Get("hardDelete") != "true" {
			t.Error("expected hardDelete=true query param")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewServiceClient(server.URL, "test-token")
	err := client.DeleteService(context.Background(), "databaseServices", "abc-123")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteServiceNotFoundIsSuccess(t *testing.T) {
	// 404 during delete means the service is already gone — treat as success.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewServiceClient(server.URL, "test-token")
	err := client.DeleteService(context.Background(), "databaseServices", "gone-id")

	if err != nil {
		t.Fatalf("expected 404 to be treated as success, got error: %v", err)
	}
}

func TestDeleteServiceServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal error`))
	}))
	defer server.Close()

	client := NewServiceClient(server.URL, "test-token")
	err := client.DeleteService(context.Background(), "databaseServices", "abc-123")

	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}
