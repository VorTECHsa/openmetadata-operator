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

// TaggableEntityType is the type of OpenMetadata entity that an
// OpenMetadataEntityTag can target. Limited to entities discovered by
// ingestion (tables, topics, schemas, etc.); the service-level entity
// itself is not tagged via this CR.
// +kubebuilder:validation:Enum=table;topic;databaseSchema;database;dashboard;mlmodel;pipeline;container;searchIndex
type TaggableEntityType string

// Taggable entity type constants. Values match the OpenMetadata "type" field
// used in entity references.
const (
	TaggableEntityTypeTable          TaggableEntityType = "table"
	TaggableEntityTypeTopic          TaggableEntityType = "topic"
	TaggableEntityTypeDatabaseSchema TaggableEntityType = "databaseSchema"
	TaggableEntityTypeDatabase       TaggableEntityType = "database"
	TaggableEntityTypeDashboard      TaggableEntityType = "dashboard"
	TaggableEntityTypeMlmodel        TaggableEntityType = "mlmodel"
	TaggableEntityTypePipeline       TaggableEntityType = "pipeline"
	TaggableEntityTypeContainer      TaggableEntityType = "container"
	TaggableEntityTypeSearchIndex    TaggableEntityType = "searchIndex"
)

// OpenMetadataEntityTagSpec defines the desired state of an
// OpenMetadataEntityTag, which applies tags to OpenMetadata entities the
// operator does not create directly (tables, topics, schemas, etc. discovered
// by ingestion).
type OpenMetadataEntityTagSpec struct {
	// Match selects the entities this CR applies to.
	Match EntityMatch `json:"match"`

	// Tag is the tag to apply to every matched entity.
	Tag TagRef `json:"tag"`

	// OpenMetadataConnectionRef is the name of the cluster-scoped OpenMetadataConnection
	// resource that defines the target OpenMetadata instance.
	// +kubebuilder:validation:MinLength=1
	OpenMetadataConnectionRef string `json:"openMetadataConnectionRef"`
}

// EntityMatch selects entities by type and FQN pattern.
type EntityMatch struct {
	// EntityType is the OpenMetadata entity type to match.
	EntityType TaggableEntityType `json:"entityType"`

	// Includes is the list of FQN patterns whose union defines the matched set.
	// Each pattern uses '*' as a wildcard (zero or more characters) and '?' for
	// a single character, applied to the entity's full FQN. Patterns should
	// typically begin with the service name to scope the match. Examples:
	// "my-svc.raw.*", "my-svc.public.events".
	// +kubebuilder:validation:MinItems=1
	Includes []string `json:"includes"`

	// Excludes is an optional list of FQN patterns to subtract from the
	// included set. Same pattern syntax as Includes.
	// +optional
	Excludes []string `json:"excludes,omitempty"`
}

// TagAssignment records a single (entity, tag) assignment the operator has
// made. Used to detect drift and to drive removal on CR deletion.
type TagAssignment struct {
	// EntityType of the tagged entity.
	EntityType TaggableEntityType `json:"entityType"`

	// EntityID is the OpenMetadata UUID of the tagged entity.
	EntityID string `json:"entityId"`

	// FullyQualifiedName of the tagged entity.
	FullyQualifiedName string `json:"fullyQualifiedName"`

	// TagFQN is the fully qualified name of the applied tag.
	TagFQN string `json:"tagFQN"`
}

// OpenMetadataEntityTagStatus defines the observed state of an OpenMetadataEntityTag.
type OpenMetadataEntityTagStatus struct {
	// TagAssignments is the set of (entity, tag) assignments the operator has
	// applied.
	// +optional
	TagAssignments []TagAssignment `json:"tagAssignments,omitempty"`

	// LastReconcileTime is the timestamp of the last successful reconciliation.
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Entity Type",type=string,JSONPath=`.spec.match.entityType`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OpenMetadataEntityTag is the Schema for the openmetadataentitytags API.
// It applies a tag to OpenMetadata entities matched by FQN pattern.
type OpenMetadataEntityTag struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec OpenMetadataEntityTagSpec `json:"spec"`

	// +optional
	Status OpenMetadataEntityTagStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OpenMetadataEntityTagList contains a list of OpenMetadataEntityTag.
type OpenMetadataEntityTagList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OpenMetadataEntityTag `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenMetadataEntityTag{}, &OpenMetadataEntityTagList{})
}
