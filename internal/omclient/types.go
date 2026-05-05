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

// EntityRef is the API payload form of an OpenMetadata EntityReference. The
// API requires id and type; other fields (name, fullyQualifiedName) are
// optional and omitted here since callers always send id+type.
type EntityRef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// ServiceRequest is the payload for PUT /api/v1/services/{endpoint}.
type ServiceRequest struct {
	Name        string         `json:"name"`
	ServiceType string         `json:"serviceType"`
	DisplayName string         `json:"displayName,omitempty"`
	Description string         `json:"description,omitempty"`
	Owners      []EntityRef    `json:"owners,omitempty"`
	Connection  map[string]any `json:"connection"`
}

// ServiceResponse is the subset of fields the operator reads from
// the OpenMetadata services API response.
type ServiceResponse struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	FullyQualifiedName string  `json:"fullyQualifiedName"`
	ServiceType        string  `json:"serviceType"`
	Version            float64 `json:"version"`
}

// PipelineRequest is the payload for PUT /v1/services/ingestionPipelines.
type PipelineRequest struct {
	Name          string         `json:"name"`
	PipelineType  string         `json:"pipelineType"`
	DisplayName   string         `json:"displayName,omitempty"`
	Description   string         `json:"description,omitempty"`
	Owners        []EntityRef    `json:"owners,omitempty"`
	SourceConfig  map[string]any `json:"sourceConfig"`
	AirflowConfig map[string]any `json:"airflowConfig"`
	Service       EntityRef      `json:"service"`
}

// PipelineResponse is the subset of fields the operator reads from
// the OpenMetadata ingestion pipelines API response.
type PipelineResponse struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	FullyQualifiedName string  `json:"fullyQualifiedName"`
	PipelineType       string  `json:"pipelineType"`
	Deployed           bool    `json:"deployed"`
	Version            float64 `json:"version"`
}

// TestCaseRequest is the payload for PUT /v1/dataQuality/testCases.
type TestCaseRequest struct {
	Name                        string                   `json:"name"`
	TestDefinition              string                   `json:"testDefinition"`
	EntityLink                  string                   `json:"entityLink"`
	DisplayName                 string                   `json:"displayName,omitempty"`
	Description                 string                   `json:"description,omitempty"`
	Owners                      []EntityRef              `json:"owners,omitempty"`
	ParameterValues             []TestCaseParameterValue `json:"parameterValues,omitempty"`
	ComputePassedFailedRowCount bool                     `json:"computePassedFailedRowCount,omitempty"`
}

// TestCaseParameterValue is a name-value pair for test parameters.
type TestCaseParameterValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// TestCaseResponse is the subset of fields the operator reads from
// the OpenMetadata test cases API response.
type TestCaseResponse struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	FullyQualifiedName string `json:"fullyQualifiedName"`
	TestSuite          struct {
		ID string `json:"id"`
	} `json:"testSuite"`
	Version float64 `json:"version"`
}

// entityIDResponse is used to extract just the ID from a generic entity lookup.
type entityIDResponse struct {
	ID string `json:"id"`
}

// EntitySummary is the minimal entity information needed to apply tags.
// Returned by SearchEntities and stored in OpenMetadataEntityTag status.
type EntitySummary struct {
	ID                 string `json:"id"`
	FullyQualifiedName string `json:"fullyQualifiedName"`
}

// searchHit represents a single hit in an OpenMetadata search response.
type searchHit struct {
	Source EntitySummary `json:"_source"`
}

// searchResponse is the relevant subset of the OpenMetadata /v1/search/query
// response.
type searchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []searchHit `json:"hits"`
	} `json:"hits"`
}

// AssetRef identifies an entity by id, type, and FQN. Used as the body of
// bulk tag-asset operations.
type AssetRef struct {
	ID                 string `json:"id"`
	Type               string `json:"type"`
	FullyQualifiedName string `json:"fullyQualifiedName,omitempty"`
}

// AddTagToAssetsRequest is the payload for bulk tag-asset add/remove.
// dryRun must be set to false to actually apply (OM defaults it to true).
type AddTagToAssetsRequest struct {
	DryRun bool       `json:"dryRun"`
	Assets []AssetRef `json:"assets"`
}
