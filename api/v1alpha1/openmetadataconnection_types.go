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

// OpenMetadataConnectionSpec defines the connection details for an OpenMetadata instance.
// Multiple managed resources (OpenMetadataService, IngestionPipeline, etc.) can
// reference the same OpenMetadataConnection by name, avoiding repetition of
// target configuration across resources.
type OpenMetadataConnectionSpec struct {
	// URL is the base URL of the OpenMetadata API (e.g. "http://openmetadata.openmetadata:8585/api").
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// AuthSecretRef points to the Kubernetes Secret containing the JWT token
	// used to authenticate with the OpenMetadata API.
	AuthSecretRef SecretReference `json:"authSecretRef"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

// OpenMetadataConnection defines how to connect to a target OpenMetadata instance.
// It is referenced by name from managed resources such as OpenMetadataService.
type OpenMetadataConnection struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec OpenMetadataConnectionSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenMetadataConnectionList contains a list of OpenMetadataConnection.
type OpenMetadataConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OpenMetadataConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenMetadataConnection{}, &OpenMetadataConnectionList{})
}
