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

package handler

import (
	"fmt"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
)

// API endpoint path segments for each service category.
const (
	endpointDatabaseServices  = "databaseServices"
	endpointMessagingServices = "messagingServices"
	endpointStorageServices   = "storageServices"
	endpointSearchServices    = "searchServices"
)

// serviceTypeRegistry maps OpenMetadata service types to their API endpoint segments.
// This registry mirrors the service type enums defined in the OpenMetadata JSON schemas
// (databaseServiceType, messagingServiceType, storageServiceType, searchServiceType).
// Add new entries here when supporting additional service types.
var serviceTypeRegistry = map[omv1alpha1.ServiceType]string{
	// Database services (databaseServiceType enum)
	omv1alpha1.ServiceTypePostgres:        endpointDatabaseServices,
	omv1alpha1.ServiceTypeAthena:          endpointDatabaseServices,
	omv1alpha1.ServiceTypeBigQuery:        endpointDatabaseServices,
	omv1alpha1.ServiceTypeBigTable:        endpointDatabaseServices,
	omv1alpha1.ServiceTypeMysql:           endpointDatabaseServices,
	omv1alpha1.ServiceTypeRedshift:        endpointDatabaseServices,
	omv1alpha1.ServiceTypeSnowflake:       endpointDatabaseServices,
	omv1alpha1.ServiceTypeTimescale:       endpointDatabaseServices,
	omv1alpha1.ServiceTypeMssql:           endpointDatabaseServices,
	omv1alpha1.ServiceTypeOracle:          endpointDatabaseServices,
	omv1alpha1.ServiceTypeHive:            endpointDatabaseServices,
	omv1alpha1.ServiceTypeImpala:          endpointDatabaseServices,
	omv1alpha1.ServiceTypePresto:          endpointDatabaseServices,
	omv1alpha1.ServiceTypeTrino:           endpointDatabaseServices,
	omv1alpha1.ServiceTypeVertica:         endpointDatabaseServices,
	omv1alpha1.ServiceTypeGlue:            endpointDatabaseServices,
	omv1alpha1.ServiceTypeMariaDB:         endpointDatabaseServices,
	omv1alpha1.ServiceTypeDruid:           endpointDatabaseServices,
	omv1alpha1.ServiceTypeDb2:             endpointDatabaseServices,
	omv1alpha1.ServiceTypeClickhouse:      endpointDatabaseServices,
	omv1alpha1.ServiceTypeDatabricks:      endpointDatabaseServices,
	omv1alpha1.ServiceTypeAzureSQL:        endpointDatabaseServices,
	omv1alpha1.ServiceTypeDynamoDB:        endpointDatabaseServices,
	omv1alpha1.ServiceTypeSingleStore:     endpointDatabaseServices,
	omv1alpha1.ServiceTypeSQLite:          endpointDatabaseServices,
	omv1alpha1.ServiceTypeDeltaLake:       endpointDatabaseServices,
	omv1alpha1.ServiceTypeSalesforce:      endpointDatabaseServices,
	omv1alpha1.ServiceTypePinotDB:         endpointDatabaseServices,
	omv1alpha1.ServiceTypeDatalake:        endpointDatabaseServices,
	omv1alpha1.ServiceTypeDomoDatabase:    endpointDatabaseServices,
	omv1alpha1.ServiceTypeQueryLog:        endpointDatabaseServices,
	omv1alpha1.ServiceTypeCustomDatabase:  endpointDatabaseServices,
	omv1alpha1.ServiceTypeDbt:             endpointDatabaseServices,
	omv1alpha1.ServiceTypeSapHana:         endpointDatabaseServices,
	omv1alpha1.ServiceTypeMongoDB:         endpointDatabaseServices,
	omv1alpha1.ServiceTypeCassandra:       endpointDatabaseServices,
	omv1alpha1.ServiceTypeCouchbase:       endpointDatabaseServices,
	omv1alpha1.ServiceTypeGreenplum:       endpointDatabaseServices,
	omv1alpha1.ServiceTypeDoris:           endpointDatabaseServices,
	omv1alpha1.ServiceTypeStarRocks:       endpointDatabaseServices,
	omv1alpha1.ServiceTypeUnityCatalog:    endpointDatabaseServices,
	omv1alpha1.ServiceTypeSAS:             endpointDatabaseServices,
	omv1alpha1.ServiceTypeIceberg:         endpointDatabaseServices,
	omv1alpha1.ServiceTypeTeradata:        endpointDatabaseServices,
	omv1alpha1.ServiceTypeSapErp:          endpointDatabaseServices,
	omv1alpha1.ServiceTypeSynapse:         endpointDatabaseServices,
	omv1alpha1.ServiceTypeExasol:          endpointDatabaseServices,
	omv1alpha1.ServiceTypeCockroach:       endpointDatabaseServices,
	omv1alpha1.ServiceTypeSSAS:            endpointDatabaseServices,
	omv1alpha1.ServiceTypeEpic:            endpointDatabaseServices,
	omv1alpha1.ServiceTypeServiceNow:      endpointDatabaseServices,
	omv1alpha1.ServiceTypeDremio:          endpointDatabaseServices,
	omv1alpha1.ServiceTypeMicrosoftFabric: endpointDatabaseServices,

	// Messaging services (messagingServiceType enum)
	omv1alpha1.ServiceTypeKafka:           endpointMessagingServices,
	omv1alpha1.ServiceTypeRedpanda:        endpointMessagingServices,
	omv1alpha1.ServiceTypeKinesis:         endpointMessagingServices,
	omv1alpha1.ServiceTypeCustomMessaging: endpointMessagingServices,

	// Storage services (storageServiceType enum)
	omv1alpha1.ServiceTypeS3:            endpointStorageServices,
	omv1alpha1.ServiceTypeADLS:          endpointStorageServices,
	omv1alpha1.ServiceTypeGCS:           endpointStorageServices,
	omv1alpha1.ServiceTypeCustomStorage: endpointStorageServices,

	// Search services (searchServiceType enum)
	omv1alpha1.ServiceTypeElasticSearch: endpointSearchServices,
	omv1alpha1.ServiceTypeOpenSearch:    endpointSearchServices,
	omv1alpha1.ServiceTypeCustomSearch:  endpointSearchServices,
}

// endpointForServiceType returns the OpenMetadata API endpoint segment for a given service type.
// Returns an error if the service type is not recognised.
func endpointForServiceType(serviceType omv1alpha1.ServiceType) (string, error) {
	endpoint, ok := serviceTypeRegistry[serviceType]
	if !ok {
		return "", fmt.Errorf("unsupported service type %q", serviceType)
	}
	return endpoint, nil
}
