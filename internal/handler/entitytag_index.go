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
	"errors"
	"fmt"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
)

// ErrUnsupportedEntityType signals a permanent spec error: the configured
// entityType has no search-index mapping in the operator. New entity types
// must be added here as we extend support.
var ErrUnsupportedEntityType = errors.New("unsupported entity type")

// entityTypeSearchIndex maps each TaggableEntityType to the OpenMetadata
// search-index name used by the /v1/search/query endpoint.
// Index names follow the OM convention `<snake_case_type>_search_index`.
var entityTypeSearchIndex = map[omv1alpha1.TaggableEntityType]string{
	omv1alpha1.TaggableEntityTypeTable:          "table_search_index",
	omv1alpha1.TaggableEntityTypeTopic:          "topic_search_index",
	omv1alpha1.TaggableEntityTypeDatabaseSchema: "database_schema_search_index",
	omv1alpha1.TaggableEntityTypeDatabase:       "database_search_index",
	omv1alpha1.TaggableEntityTypeDashboard:      "dashboard_search_index",
	omv1alpha1.TaggableEntityTypeMlmodel:        "mlmodel_search_index",
	omv1alpha1.TaggableEntityTypePipeline:       "pipeline_search_index",
	omv1alpha1.TaggableEntityTypeContainer:      "container_search_index",
	omv1alpha1.TaggableEntityTypeSearchIndex:    "search_entity_search_index",
}

// resolveEntitySearchIndex returns the search-index name for the given entity
// type, or ErrUnsupportedEntityType if it has no mapping.
func resolveEntitySearchIndex(t omv1alpha1.TaggableEntityType) (string, error) {
	if idx, ok := entityTypeSearchIndex[t]; ok {
		return idx, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnsupportedEntityType, t)
}
