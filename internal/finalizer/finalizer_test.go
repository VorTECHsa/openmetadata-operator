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

package finalizer

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsPresentOnCleanObject(t *testing.T) {
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	if IsPresent(obj) {
		t.Error("expected IsPresent to return false on clean object")
	}
}

func TestIsPresentWithFinalizer(t *testing.T) {
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Namespace:  "default",
			Finalizers: []string{Name},
		},
	}
	if !IsPresent(obj) {
		t.Error("expected IsPresent to return true when finalizer is set")
	}
}

func TestEnsurePresentAdds(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		Build()

	added, err := EnsurePresent(context.Background(), c, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("expected added=true")
	}
	if !IsPresent(obj) {
		t.Error("expected finalizer to be present after EnsurePresent")
	}
}

func TestEnsurePresentIdempotent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Namespace:  "default",
			Finalizers: []string{Name},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		Build()

	added, err := EnsurePresent(context.Background(), c, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added {
		t.Error("expected added=false when finalizer already present")
	}
}

func TestEnsureAbsentRemoves(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Namespace:  "default",
			Finalizers: []string{Name},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		Build()

	if err := EnsureAbsent(context.Background(), c, obj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if IsPresent(obj) {
		t.Error("expected finalizer to be absent after EnsureAbsent")
	}
}

func TestEnsureAbsentIdempotent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		Build()

	if err := EnsureAbsent(context.Background(), c, obj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
