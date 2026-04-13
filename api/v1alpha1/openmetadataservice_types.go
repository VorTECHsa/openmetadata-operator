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
	"k8s.io/apimachinery/pkg/runtime"
)

// OpenMetadataServiceSpec defines the desired state of an OpenMetadata service registration.
type OpenMetadataServiceSpec struct {
	// ForOpenMetadata contains the fields forwarded to the OpenMetadata API.
	ForOpenMetadata ServiceOMSpec `json:"forOpenMetadata"`

	// OpenMetadataConnectionRef is the name of the cluster-scoped OpenMetadataConnection
	// resource that defines the target OpenMetadata instance.
	// +kubebuilder:validation:MinLength=1
	OpenMetadataConnectionRef string `json:"openMetadataConnectionRef"`
}

// ServiceOMSpec holds the OpenMetadata API fields for a service registration.
type ServiceOMSpec struct {
	// ServiceType is the OpenMetadata service type (e.g. Postgres, Athena, Kafka).
	// The operator derives the correct API endpoint from this value automatically.
	ServiceType ServiceType `json:"serviceType"`

	// DisplayName is a human-readable name shown in the OpenMetadata UI.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Description is a markdown-formatted description of the service.
	// +optional
	Description string `json:"description,omitempty"`

	// Connection holds the opaque connection configuration forwarded to OpenMetadata.
	Connection ConnectionSpec `json:"connection"`
}

// ConnectionSpec wraps the opaque connection configuration.
type ConnectionSpec struct {
	// Config is the connection configuration map passed through to the OpenMetadata API
	// after resolving any secret references. Each field must use one of:
	//   {"value": <literal>}       -- unwrapped and passed through as-is
	//   {"valueFrom": {"secretKeyRef": {"name": "<secret>", "key": "<key>"}}}
	//                              -- resolved from the referenced Kubernetes Secret
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Config runtime.RawExtension `json:"config"`
}

// SecretReference identifies a specific key within a Kubernetes Secret.
type SecretReference struct {
	// Name of the Secret.
	Name string `json:"name"`

	// Namespace of the Secret. Required for cross-namespace access (e.g. reading
	// a secret from the openmetadata namespace).
	Namespace string `json:"namespace"`

	// Key within the Secret's data field.
	Key string `json:"key"`
}

// OpenMetadataServiceStatus defines the observed state of an OpenMetadata service registration.
type OpenMetadataServiceStatus struct {
	// OpenMetadataID is the UUID assigned by OpenMetadata when the service was created.
	// +optional
	OpenMetadataID string `json:"openmetadataId,omitempty"`

	// FullyQualifiedName is the FQN as stored in OpenMetadata.
	// +optional
	FullyQualifiedName string `json:"fullyQualifiedName,omitempty"`

	// LastReconcileTime is the timestamp of the last successful reconciliation.
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// RemoteVersion is the entity version last seen in OpenMetadata,
	// serialised as a string (e.g. "0.1"). Used to detect external drift
	// (e.g. manual changes in the OM UI).
	// +optional
	RemoteVersion string `json:"remoteVersion,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Service Type",type=string,JSONPath=`.spec.forOpenMetadata.serviceType`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="OM ID",type=string,JSONPath=`.status.openmetadataId`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OpenMetadataService is the Schema for the openmetadataservices API.
// It declaratively manages a service registration in OpenMetadata.
type OpenMetadataService struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec OpenMetadataServiceSpec `json:"spec"`

	// +optional
	Status OpenMetadataServiceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OpenMetadataServiceList contains a list of OpenMetadataService.
type OpenMetadataServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OpenMetadataService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenMetadataService{}, &OpenMetadataServiceList{})
}
