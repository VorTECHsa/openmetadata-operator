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

// Package condition provides pure helper functions for building and querying
// Kubernetes status conditions on OpenMetadata custom resources.
package condition

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
)

// SetReady sets (or updates) the Ready condition on the supplied conditions
// slice. It is a pure data operation — no API calls are made.
func SetReady(conditions *[]metav1.Condition, generation int64, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               omv1alpha1.ConditionTypeReady,
		Status:             status,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// FindReady returns a pointer to the Ready condition if present, or nil.
func FindReady(conditions []metav1.Condition) *metav1.Condition {
	return meta.FindStatusCondition(conditions, omv1alpha1.ConditionTypeReady)
}
