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

// PipelineType represents the type of an OpenMetadata ingestion pipeline.
// +kubebuilder:validation:Enum=metadata;usage;lineage;profiler;autoClassification;TestSuite;dataInsight;elasticSearchReindex;dbt;application
type PipelineType string

// Pipeline type constants matching the OpenMetadata pipelineType enum.
const (
	PipelineTypeMetadata             PipelineType = "metadata"
	PipelineTypeUsage                PipelineType = "usage"
	PipelineTypeLineage              PipelineType = "lineage"
	PipelineTypeProfiler             PipelineType = "profiler"
	PipelineTypeAutoClassification   PipelineType = "autoClassification"
	PipelineTypeTestSuite            PipelineType = "TestSuite"
	PipelineTypeDataInsight          PipelineType = "dataInsight"
	PipelineTypeElasticSearchReindex PipelineType = "elasticSearchReindex"
	PipelineTypeDbt                  PipelineType = "dbt"
	PipelineTypeApplication          PipelineType = "application"
)

// EntityType represents the type of an entity referenced in OpenMetadata.
// +kubebuilder:validation:Enum=databaseService;messagingService;dashboardService;pipelineService;mlmodelService;storageService;searchService;metadataService;apiService;testSuite
type EntityType string

// Entity type constants for service entity references.
const (
	EntityTypeDatabaseService  EntityType = "databaseService"
	EntityTypeMessagingService EntityType = "messagingService"
	EntityTypeDashboardService EntityType = "dashboardService"
	EntityTypePipelineService  EntityType = "pipelineService"
	EntityTypeMlmodelService   EntityType = "mlmodelService"
	EntityTypeStorageService   EntityType = "storageService"
	EntityTypeSearchService    EntityType = "searchService"
	EntityTypeMetadataService  EntityType = "metadataService"
	EntityTypeAPIService       EntityType = "apiService"
	EntityTypeTestSuite        EntityType = "testSuite"
)

// IngestionPipelineSpec defines the desired state of an IngestionPipeline.
type IngestionPipelineSpec struct {
	// ForOpenMetadata contains the fields forwarded to the OpenMetadata API.
	ForOpenMetadata IngestionPipelineOMSpec `json:"forOpenMetadata"`

	// OpenMetadataConnectionRef is the name of the cluster-scoped OpenMetadataConnection
	// resource that defines the target OpenMetadata instance.
	// +kubebuilder:validation:MinLength=1
	OpenMetadataConnectionRef string `json:"openMetadataConnectionRef"`
}

// IngestionPipelineOMSpec holds the OpenMetadata API fields for an ingestion pipeline.
type IngestionPipelineOMSpec struct {
	// PipelineType is the OpenMetadata pipeline type.
	PipelineType PipelineType `json:"pipelineType"`

	// Service identifies the service entity this pipeline belongs to in OpenMetadata.
	// For metadata/profiler/usage/lineage pipelines: reference a databaseService, messagingService, etc.
	// For TestSuite pipelines: reference a testSuite.
	// Matches the "service" field in the OM API.
	Service EntityReference `json:"service"`

	// SourceConfig holds the opaque pipeline-specific source configuration
	// forwarded to the OpenMetadata API. The shape depends on pipelineType.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Required
	SourceConfig runtime.RawExtension `json:"sourceConfig"`

	// AirflowConfig holds Airflow DAG scheduling configuration.
	AirflowConfig AirflowConfig `json:"airflowConfig"`

	// DisplayName is a human-readable name shown in the OpenMetadata UI.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Description of the pipeline.
	// +optional
	Description string `json:"description,omitempty"`
}

// EntityReference identifies an entity in OpenMetadata by FQN and type.
type EntityReference struct {
	// FullyQualifiedName of the referenced entity.
	// +kubebuilder:validation:MinLength=1
	FullyQualifiedName string `json:"fullyQualifiedName"`

	// Type of the referenced entity (e.g. "databaseService", "testSuite").
	Type EntityType `json:"type"`
}

// AirflowConfig holds Airflow DAG scheduling parameters.
type AirflowConfig struct {
	// ScheduleInterval is a cron expression for the pipeline schedule.
	// +optional
	ScheduleInterval string `json:"scheduleInterval,omitempty"`

	// StartDate for the Airflow DAG in ISO 8601 format.
	// +optional
	StartDate *metav1.Time `json:"startDate,omitempty"`

	// Concurrency controls how many tasks can run in parallel.
	// +optional
	// +kubebuilder:default=1
	Concurrency int `json:"concurrency,omitempty"`

	// Retries is the number of times to retry on failure.
	// +optional
	// +kubebuilder:default=0
	Retries int `json:"retries,omitempty"`

	// RetryDelay is the number of seconds between retries.
	// +optional
	// +kubebuilder:default=300
	RetryDelay int `json:"retryDelay,omitempty"`

	// PausePipeline controls whether the pipeline is paused after deployment.
	// +optional
	PausePipeline bool `json:"pausePipeline,omitempty"`

	// PipelineTimezone is an IANA timezone name for the pipeline schedule (e.g. "UTC", "Europe/London").
	// +optional
	// +kubebuilder:default="UTC"
	PipelineTimezone string `json:"pipelineTimezone,omitempty"`

	// PipelineCatchup controls whether Airflow should catch up on missed runs.
	// +optional
	PipelineCatchup bool `json:"pipelineCatchup,omitempty"`

	// MaxActiveRuns is the maximum number of active DAG runs.
	// +optional
	// +kubebuilder:default=1
	MaxActiveRuns int `json:"maxActiveRuns,omitempty"`
}

// IngestionPipelineStatus defines the observed state of an IngestionPipeline.
type IngestionPipelineStatus struct {
	// OpenMetadataID is the UUID assigned by OpenMetadata.
	// +optional
	OpenMetadataID string `json:"openmetadataId,omitempty"`

	// FullyQualifiedName as stored in OpenMetadata.
	// +optional
	FullyQualifiedName string `json:"fullyQualifiedName,omitempty"`

	// Deployed indicates whether the pipeline has been successfully deployed to Airflow.
	// +optional
	Deployed bool `json:"deployed,omitempty"`

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
// +kubebuilder:printcolumn:name="Pipeline Type",type=string,JSONPath=`.spec.forOpenMetadata.pipelineType`
// +kubebuilder:printcolumn:name="Deployed",type=boolean,JSONPath=`.status.deployed`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// IngestionPipeline is the Schema for the ingestionpipelines API.
// It declaratively manages an ingestion pipeline in OpenMetadata.
type IngestionPipeline struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec IngestionPipelineSpec `json:"spec"`

	// +optional
	Status IngestionPipelineStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// IngestionPipelineList contains a list of IngestionPipeline.
type IngestionPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []IngestionPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IngestionPipeline{}, &IngestionPipelineList{})
}
