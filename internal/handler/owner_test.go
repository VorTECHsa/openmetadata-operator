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
	"strings"
	"testing"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

// stubResolver is a minimal ownerResolver for testing.
type stubResolver struct {
	entities map[string]string
}

func (s *stubResolver) GetEntityByName(_ context.Context, path, fqn string) (string, error) {
	if id, ok := s.entities[path+"/"+fqn]; ok {
		return id, nil
	}
	return "", &omclient.APIError{StatusCode: 404, Body: "not found"}
}

func TestResolveOwners(t *testing.T) {
	resolver := &stubResolver{
		entities: map[string]string{
			"users/alice":         "user-uuid-1",
			"teams/platform-team": "team-uuid-2",
		},
	}

	tests := []struct {
		name    string
		input   []omv1alpha1.EntityReference
		want    []omclient.EntityRef
		errSubs string
	}{
		{
			name:  "nil input returns nil",
			input: nil,
			want:  nil,
		},
		{
			name:  "empty input returns nil",
			input: []omv1alpha1.EntityReference{},
			want:  nil,
		},
		{
			name: "single user resolves",
			input: []omv1alpha1.EntityReference{
				{FullyQualifiedName: "alice", Type: omv1alpha1.EntityTypeUser},
			},
			want: []omclient.EntityRef{
				{ID: "user-uuid-1", Type: string(omv1alpha1.EntityTypeUser)},
			},
		},
		{
			name: "single team resolves",
			input: []omv1alpha1.EntityReference{
				{FullyQualifiedName: "platform-team", Type: omv1alpha1.EntityTypeTeam},
			},
			want: []omclient.EntityRef{
				{ID: "team-uuid-2", Type: string(omv1alpha1.EntityTypeTeam)},
			},
		},
		{
			name: "mixed users and teams resolve in order",
			input: []omv1alpha1.EntityReference{
				{FullyQualifiedName: "platform-team", Type: omv1alpha1.EntityTypeTeam},
				{FullyQualifiedName: "alice", Type: omv1alpha1.EntityTypeUser},
			},
			want: []omclient.EntityRef{
				{ID: "team-uuid-2", Type: string(omv1alpha1.EntityTypeTeam)},
				{ID: "user-uuid-1", Type: string(omv1alpha1.EntityTypeUser)},
			},
		},
		{
			name: "unsupported type returns error",
			input: []omv1alpha1.EntityReference{
				{FullyQualifiedName: "some-db", Type: omv1alpha1.EntityTypeDatabaseService},
			},
			errSubs: "unsupported owner type",
		},
		{
			name: "unknown owner returns wrapped error",
			input: []omv1alpha1.EntityReference{
				{FullyQualifiedName: "ghost", Type: omv1alpha1.EntityTypeUser},
			},
			errSubs: `resolving owner user "ghost"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveOwners(context.Background(), resolver, tt.input)
			if tt.errSubs != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errSubs)
				}
				if !strings.Contains(err.Error(), tt.errSubs) {
					t.Fatalf("expected error containing %q, got %q", tt.errSubs, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d owners, got %d", len(tt.want), len(got))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: expected %+v, got %+v", i, tt.want[i], got[i])
				}
			}
		})
	}
}
