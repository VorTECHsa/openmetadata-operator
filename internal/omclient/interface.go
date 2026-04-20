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

import "context"

// ServiceClient defines the operations the operator needs against the OpenMetadata
// services API. Extracting an interface allows the controller to be tested with
// mock implementations.
type ServiceClient interface {
	// UpsertService creates or updates a service via PUT.
	UpsertService(ctx context.Context, endpoint string, req ServiceRequest) (*ServiceResponse, error)

	// GetServiceByName retrieves a service by its fully qualified name.
	// Returns an *APIError with status 404 if the service does not exist.
	GetServiceByName(ctx context.Context, endpoint, fqn string) (*ServiceResponse, error)

	// DeleteService deletes a service by ID with recursive and hard delete.
	DeleteService(ctx context.Context, endpoint, id string) error

	// GetEntityByName retrieves any entity by type path and FQN, returning its ID.
	// Used to resolve owner references (users/teams) to OM UUIDs.
	GetEntityByName(ctx context.Context, entityTypePath, fqn string) (string, error)
}

// PipelineClient defines operations for managing ingestion pipelines.
type PipelineClient interface {
	// UpsertPipeline creates or updates an ingestion pipeline via PUT.
	UpsertPipeline(ctx context.Context, req PipelineRequest) (*PipelineResponse, error)

	// GetPipelineByFQN retrieves a pipeline by fully qualified name.
	// Returns an *APIError with status 404 if the pipeline does not exist.
	GetPipelineByFQN(ctx context.Context, fqn string) (*PipelineResponse, error)

	// DeployPipeline triggers Airflow DAG deployment for the given pipeline ID.
	DeployPipeline(ctx context.Context, id string) error

	// DeletePipeline deletes a pipeline by ID with hard delete.
	DeletePipeline(ctx context.Context, id string) error

	// GetEntityByName retrieves any entity by type path and FQN, returning its ID.
	// Used to resolve FQN-based references to OM UUIDs.
	GetEntityByName(ctx context.Context, entityTypePath, fqn string) (string, error)
}

// TestCaseClient defines operations for managing test cases.
type TestCaseClient interface {
	// UpsertTestCase creates or updates a test case via PUT.
	UpsertTestCase(ctx context.Context, req TestCaseRequest) (*TestCaseResponse, error)

	// GetTestCaseByFQN retrieves a test case by fully qualified name.
	// Returns an *APIError with status 404 if the test case does not exist.
	GetTestCaseByFQN(ctx context.Context, fqn string) (*TestCaseResponse, error)

	// DeleteTestCase deletes a test case by ID with hard delete.
	DeleteTestCase(ctx context.Context, id string) error

	// GetEntityByName retrieves any entity by type path and FQN, returning its ID.
	// Used to resolve owner references (users/teams) to OM UUIDs.
	GetEntityByName(ctx context.Context, entityTypePath, fqn string) (string, error)
}
