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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenMetadataTestCaseSpec defines the desired state of an OpenMetadataTestCase.
type OpenMetadataTestCaseSpec struct {
	// ForOpenMetadata contains the fields forwarded to the OpenMetadata API.
	ForOpenMetadata TestCaseOMSpec `json:"forOpenMetadata"`

	// OpenMetadataConnectionRef is the name of the cluster-scoped OpenMetadataConnection
	// resource that defines the target OpenMetadata instance.
	// +kubebuilder:validation:MinLength=1
	OpenMetadataConnectionRef string `json:"openMetadataConnectionRef"`
}

// TestCaseOMSpec holds the OpenMetadata API fields for a test case.
type TestCaseOMSpec struct {
	// TestDefinition is the FQN of the test definition (e.g. "columnValuesToBeNotNull").
	// +kubebuilder:validation:MinLength=1
	TestDefinition string `json:"testDefinition"`

	// EntityLink identifies the entity being tested.
	// Format: <#E::table::service.database.schema.table> for table tests
	//         <#E::table::service.database.schema.table::columns::colName> for column tests
	// +kubebuilder:validation:MinLength=1
	EntityLink string `json:"entityLink"`

	// ParameterValues are the test-specific parameters.
	// +optional
	ParameterValues []TestCaseParameterValue `json:"parameterValues,omitempty"`

	// DisplayName is a human-readable name shown in the OpenMetadata UI.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Description of the test case.
	// +optional
	Description string `json:"description,omitempty"`

	// ComputePassedFailedRowCount controls whether to compute row counts.
	// +optional
	ComputePassedFailedRowCount bool `json:"computePassedFailedRowCount,omitempty"`

	// Owners is the list of users and/or teams that own this test case in OpenMetadata.
	// Each entry references a user or team by fullyQualifiedName; the operator
	// resolves these to UUIDs at reconcile time.
	// +optional
	Owners []EntityReference `json:"owners,omitempty"`
}

// TestCaseParameterValue is a name-value pair for test parameters.
type TestCaseParameterValue struct {
	// Name of the parameter.
	Name string `json:"name"`

	// Value of the parameter.
	Value string `json:"value"`
}

// OpenMetadataTestCaseStatus defines the observed state of an OpenMetadataTestCase.
type OpenMetadataTestCaseStatus struct {
	// OpenMetadataID is the UUID assigned by OpenMetadata.
	// +optional
	OpenMetadataID string `json:"openmetadataId,omitempty"`

	// FullyQualifiedName as stored in OpenMetadata.
	// +optional
	FullyQualifiedName string `json:"fullyQualifiedName,omitempty"`

	// TestSuiteID is the UUID of the auto-created basic test suite.
	// +optional
	TestSuiteID string `json:"testSuiteId,omitempty"`

	// LastReconcileTime is the timestamp of the last successful reconciliation.
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// RemoteVersion is the entity version last seen in OpenMetadata.
	// +optional
	RemoteVersion string `json:"remoteVersion,omitempty"`

	// Conditions represent the latest available observations.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Test Definition",type=string,JSONPath=`.spec.forOpenMetadata.testDefinition`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OpenMetadataTestCase is the Schema for the openmetadatatestcases API.
// It declaratively manages a test case in OpenMetadata.
type OpenMetadataTestCase struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec OpenMetadataTestCaseSpec `json:"spec"`

	// +optional
	Status OpenMetadataTestCaseStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OpenMetadataTestCaseList contains a list of OpenMetadataTestCase.
type OpenMetadataTestCaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OpenMetadataTestCase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenMetadataTestCase{}, &OpenMetadataTestCaseList{})
}
