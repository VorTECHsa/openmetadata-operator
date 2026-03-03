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

func TestUpsertTestCase(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/v1/dataQuality/testCases" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req TestCaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if req.Name != "orders-status-not-null" {
			t.Errorf("expected name 'orders-status-not-null', got %q", req.Name)
		}
		if req.TestDefinition != "columnValuesToBeNotNull" {
			t.Errorf("expected testDefinition 'columnValuesToBeNotNull', got %q", req.TestDefinition)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(TestCaseResponse{
			ID:                 "tc-uuid-1",
			Name:               "orders-status-not-null",
			FullyQualifiedName: "my-postgres.my_db.public.orders.orders-status-not-null",
			TestSuite: struct {
				ID string `json:"id"`
			}{ID: "suite-uuid-1"},
			Version: 0.1,
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := NewTestCaseClient(server.URL, "test-token")
	resp, err := client.UpsertTestCase(context.Background(), TestCaseRequest{
		Name:           "orders-status-not-null",
		TestDefinition: "columnValuesToBeNotNull",
		EntityLink:     "<#E::table::my-postgres.my_db.public.orders::columns::status>",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "tc-uuid-1" {
		t.Errorf("expected ID 'tc-uuid-1', got %q", resp.ID)
	}
	if resp.TestSuite.ID != "suite-uuid-1" {
		t.Errorf("expected TestSuite.ID 'suite-uuid-1', got %q", resp.TestSuite.ID)
	}
}

func TestUpsertTestCaseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":400,"message":"invalid payload"}`))
	}))
	defer server.Close()

	client := NewTestCaseClient(server.URL, "test-token")
	_, err := client.UpsertTestCase(context.Background(), TestCaseRequest{Name: "bad"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetTestCaseByFQN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/dataQuality/testCases/name/my-postgres.my_db.public.orders.orders-status-not-null" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(TestCaseResponse{
			ID:                 "tc-uuid-1",
			Name:               "orders-status-not-null",
			FullyQualifiedName: "my-postgres.my_db.public.orders.orders-status-not-null",
			Version:            0.2,
		})
	}))
	defer server.Close()

	client := NewTestCaseClient(server.URL, "test-token")
	resp, err := client.GetTestCaseByFQN(context.Background(), "my-postgres.my_db.public.orders.orders-status-not-null")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.ID != "tc-uuid-1" {
		t.Errorf("expected ID 'tc-uuid-1', got %q", resp.ID)
	}
}

func TestGetTestCaseByFQNNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
	}))
	defer server.Close()

	client := NewTestCaseClient(server.URL, "test-token")
	resp, err := client.GetTestCaseByFQN(context.Background(), "nonexistent")

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

func TestDeleteTestCase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/v1/dataQuality/testCases/tc-uuid-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if !r.URL.Query().Has("hardDelete") {
			t.Error("expected hardDelete query param")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewTestCaseClient(server.URL, "test-token")
	err := client.DeleteTestCase(context.Background(), "tc-uuid-1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteTestCaseNotFoundIsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewTestCaseClient(server.URL, "test-token")
	err := client.DeleteTestCase(context.Background(), "gone-id")

	if err != nil {
		t.Fatalf("expected 404 to be treated as success, got error: %v", err)
	}
}
