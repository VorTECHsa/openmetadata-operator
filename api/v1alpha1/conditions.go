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

// Condition type constants for OpenMetadata custom resources.
const (
	// ConditionTypeReady indicates whether the resource is successfully
	// reconciled with the remote OpenMetadata instance.
	ConditionTypeReady = "Ready"
)

// Condition reason constants for OpenMetadata custom resources.
const (
	// ReasonCreated indicates the resource was newly created in OpenMetadata.
	ReasonCreated = "Created"

	// ReasonUpdated indicates the resource was updated in OpenMetadata.
	ReasonUpdated = "Updated"

	// ReasonInSync indicates the resource is in sync with OpenMetadata.
	ReasonInSync = "InSync"

	// ReasonDriftCorrected indicates external drift was detected and corrected.
	ReasonDriftCorrected = "DriftCorrected"

	// ReasonDeleted indicates the resource was deleted from OpenMetadata.
	ReasonDeleted = "Deleted"

	// ReasonUpsertFailed indicates an upsert operation against OpenMetadata failed.
	ReasonUpsertFailed = "UpsertFailed"

	// ReasonDeletionFailed indicates a deletion operation against OpenMetadata failed.
	ReasonDeletionFailed = "DeletionFailed"

	// ReasonQueryFailed indicates a query to OpenMetadata failed.
	ReasonQueryFailed = "QueryFailed"

	// ReasonUnsupportedServiceType indicates the service type is not in the registry.
	ReasonUnsupportedServiceType = "UnsupportedServiceType"

	// ReasonConnectionNotFound indicates the referenced OpenMetadataConnection could not be found.
	ReasonConnectionNotFound = "ConnectionNotFound"

	// ReasonAuthTokenUnavailable indicates the auth token secret could not be read.
	ReasonAuthTokenUnavailable = "AuthTokenUnavailable"

	// ReasonInvalidConfig indicates the connection config is malformed.
	ReasonInvalidConfig = "InvalidConfig"

	// ReasonSecretResolutionFailed indicates a secret reference could not be resolved.
	ReasonSecretResolutionFailed = "SecretResolutionFailed"

	// ReasonDeployed indicates the pipeline was successfully deployed to Airflow.
	ReasonDeployed = "Deployed"

	// ReasonDeployFailed indicates the deployment to Airflow failed.
	ReasonDeployFailed = "DeployFailed"

	// ReasonServiceNotFound indicates the referenced service entity could not be found in OpenMetadata.
	ReasonServiceNotFound = "ServiceNotFound"

	// ReasonOwnerResolutionFailed indicates an owner could not be resolved to an OpenMetadata UUID.
	ReasonOwnerResolutionFailed = "OwnerResolutionFailed"
)
