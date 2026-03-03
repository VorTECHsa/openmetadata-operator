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

package secretresolver

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newFakeResolver creates a Resolver backed by a fake Kubernetes client
// pre-populated with the given secrets. No cluster needed.
func newFakeResolver(secrets ...*corev1.Secret) *Resolver {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	objects := make([]runtime.Object, len(secrets))
	for i, s := range secrets {
		objects[i] = s
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objects...).
		Build()

	return NewResolver(client, "test-ns")
}

// valueFromSecretKeyRef is a test helper that builds the valueFrom structure.
func valueFromSecretKeyRef(secretName, key string) map[string]any {
	return map[string]any{
		"valueFrom": map[string]any{
			"secretKeyRef": map[string]any{
				"name": secretName,
				"key":  key,
			},
		},
	}
}

// valueWrapper is a test helper that builds a literal value wrapper.
func valueWrapper(v any) map[string]any {
	return map[string]any{"value": v}
}

func TestResolveSimpleSecretRef(t *testing.T) {
	resolver := newFakeResolver(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "test-ns"},
		Data:       map[string][]byte{"password": []byte("s3cret")},
	})

	input := valueFromSecretKeyRef("my-secret", "password")

	result, err := resolver.Resolve(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	str, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if str != "s3cret" {
		t.Errorf("expected 's3cret', got %q", str)
	}
}

func TestResolveValueWrapper(t *testing.T) {
	resolver := newFakeResolver()

	input := valueWrapper("plain-text")

	result, err := resolver.Resolve(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	str, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if str != "plain-text" {
		t.Errorf("expected 'plain-text', got %q", str)
	}
}

func TestResolveNestedConfig(t *testing.T) {
	resolver := newFakeResolver(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-output", Namespace: "test-ns"},
			Data: map[string][]byte{
				"endpoint": []byte("db.example.com:5432"),
				"username": []byte("admin"),
				"password": []byte("hunter2"),
			},
		},
	)

	// Mimics a real Postgres connection config with the new format.
	input := map[string]any{
		"type":     valueWrapper("Postgres"),
		"hostPort": valueFromSecretKeyRef("db-output", "endpoint"),
		"database": valueWrapper("mydb"),
		"username": valueFromSecretKeyRef("db-output", "username"),
		"authType": map[string]any{
			"password": valueFromSecretKeyRef("db-output", "password"),
		},
		"sslMode": valueWrapper("prefer"),
	}

	result, err := resolver.Resolve(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolved := result.(map[string]any)

	if resolved["type"] != "Postgres" {
		t.Errorf("expected 'Postgres', got %v", resolved["type"])
	}
	if resolved["hostPort"] != "db.example.com:5432" {
		t.Errorf("expected 'db.example.com:5432', got %v", resolved["hostPort"])
	}
	if resolved["database"] != "mydb" {
		t.Errorf("expected 'mydb', got %v", resolved["database"])
	}
	if resolved["username"] != "admin" {
		t.Errorf("expected 'admin', got %v", resolved["username"])
	}
	if resolved["sslMode"] != "prefer" {
		t.Errorf("expected 'prefer', got %v", resolved["sslMode"])
	}

	authType := resolved["authType"].(map[string]any)
	if authType["password"] != "hunter2" {
		t.Errorf("expected 'hunter2', got %v", authType["password"])
	}
}

func TestResolveSliceWithSecretRefs(t *testing.T) {
	resolver := newFakeResolver(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "test-ns"},
		Data:       map[string][]byte{"token": []byte("abc123")},
	})

	input := []any{
		"plain-value",
		valueFromSecretKeyRef("my-secret", "token"),
	}

	result, err := resolver.Resolve(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolved := result.([]any)
	if resolved[0] != "plain-value" {
		t.Errorf("expected 'plain-value', got %v", resolved[0])
	}
	if resolved[1] != "abc123" {
		t.Errorf("expected 'abc123', got %v", resolved[1])
	}
}

func TestResolveMapWithExtraFieldsIsNotValueFrom(t *testing.T) {
	resolver := newFakeResolver()

	// A map with valueFrom + another field should NOT be treated
	// as a secret reference (the len==1 guard prevents false positives).
	input := map[string]any{
		"valueFrom":  map[string]any{"secretKeyRef": map[string]any{"name": "s", "key": "k"}},
		"extraField": "something",
	}

	result, err := resolver.Resolve(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolved := result.(map[string]any)
	if _, ok := resolved["valueFrom"]; !ok {
		t.Error("expected valueFrom to remain in map (not treated as a reference)")
	}
}

func TestResolveMissingSecret(t *testing.T) {
	resolver := newFakeResolver() // No secrets exist

	input := valueFromSecretKeyRef("nonexistent", "password")

	_, err := resolver.Resolve(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}
}

func TestResolveMissingKey(t *testing.T) {
	resolver := newFakeResolver(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "test-ns"},
		Data:       map[string][]byte{"other-key": []byte("value")},
	})

	input := valueFromSecretKeyRef("my-secret", "nonexistent-key")

	_, err := resolver.Resolve(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestResolvePrimitives(t *testing.T) {
	resolver := newFakeResolver()

	tests := []struct {
		name  string
		input any
	}{
		{"string", "hello"},
		{"float", 42.0},
		{"bool", true},
		{"nil", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.Resolve(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.input {
				t.Errorf("expected %v, got %v", tt.input, result)
			}
		})
	}
}

func TestResolveValueWrapperWithComplexValue(t *testing.T) {
	resolver := newFakeResolver()

	// value: wraps an object literal -- should be returned as-is, not recursed.
	input := valueWrapper(map[string]any{
		"valueFrom": "this is a literal, not a reference",
	})

	result, err := resolver.Resolve(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolved := result.(map[string]any)
	if resolved["valueFrom"] != "this is a literal, not a reference" {
		t.Errorf("expected literal map, got %v", resolved)
	}
}
