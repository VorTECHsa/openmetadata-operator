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
	"context"
	"errors"
	"fmt"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

// ErrUnsupportedOwnerType signals a permanent spec error: an owner has a type
// other than "user" or "team". Distinguished from transient errors so callers
// can skip requeue.
var ErrUnsupportedOwnerType = errors.New("unsupported owner type")

// ownerResolver is the narrow interface needed by resolveOwners
type ownerResolver interface {
	GetEntityByName(ctx context.Context, entityTypePath, fqn string) (string, error)
}

// ownerTypePath maps owner EntityType values to OpenMetadata API path segments.
var ownerTypePath = map[omv1alpha1.EntityType]string{
	omv1alpha1.EntityTypeUser: "users",
	omv1alpha1.EntityTypeTeam: "teams",
}

// resolveOwners converts user-facing owner references (FQN + type) to API
// owner references (id + type) by resolving each FQN via GetEntityByName.
// Returns nil for an empty input so callers omit the owners field entirely.
// Wraps the underlying error so callers can use errors.Is(err, ErrUnsupportedOwnerType)
// to distinguish permanent spec errors from transient ones.
func resolveOwners(ctx context.Context, client ownerResolver, owners []omv1alpha1.EntityReference) ([]omclient.EntityRef, error) {
	if len(owners) == 0 {
		return nil, nil
	}
	resolved := make([]omclient.EntityRef, 0, len(owners))
	for _, o := range owners {
		path, ok := ownerTypePath[o.Type]
		if !ok {
			return nil, fmt.Errorf("%w %q (must be user or team)", ErrUnsupportedOwnerType, o.Type)
		}
		id, err := client.GetEntityByName(ctx, path, o.FullyQualifiedName)
		if err != nil {
			return nil, fmt.Errorf("resolving owner %s %q: %w", o.Type, o.FullyQualifiedName, err)
		}
		resolved = append(resolved, omclient.EntityRef{ID: id, Type: string(o.Type)})
	}
	return resolved, nil
}
