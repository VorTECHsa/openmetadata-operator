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

package condition

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
)

func TestSetReady(t *testing.T) {
	var conditions []metav1.Condition
	SetReady(&conditions, 1, metav1.ConditionTrue, "InSync", "Service is in sync")

	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conditions))
	}
	c := conditions[0]
	if c.Type != omv1alpha1.ConditionTypeReady {
		t.Errorf("expected type %q, got %q", omv1alpha1.ConditionTypeReady, c.Type)
	}
	if c.Status != metav1.ConditionTrue {
		t.Errorf("expected status %q, got %q", metav1.ConditionTrue, c.Status)
	}
	if c.Reason != "InSync" {
		t.Errorf("expected reason %q, got %q", "InSync", c.Reason)
	}
	if c.Message != "Service is in sync" {
		t.Errorf("expected message %q, got %q", "Service is in sync", c.Message)
	}
	if c.ObservedGeneration != 1 {
		t.Errorf("expected generation 1, got %d", c.ObservedGeneration)
	}
}

func TestSetReadyOverwritesExisting(t *testing.T) {
	var conditions []metav1.Condition
	SetReady(&conditions, 1, metav1.ConditionFalse, "Failing", "oops")
	SetReady(&conditions, 2, metav1.ConditionTrue, "InSync", "ok")

	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition after overwrite, got %d", len(conditions))
	}
	if conditions[0].Status != metav1.ConditionTrue {
		t.Errorf("expected True after overwrite, got %s", conditions[0].Status)
	}
	if conditions[0].Reason != "InSync" {
		t.Errorf("expected reason InSync, got %s", conditions[0].Reason)
	}
}
