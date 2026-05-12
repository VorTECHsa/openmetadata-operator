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
	"sort"
	"testing"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

func TestDiffAssets(t *testing.T) {
	matched := []omclient.EntitySummary{
		{ID: "id-1", FullyQualifiedName: "svc.raw.t1"},
		{ID: "id-2", FullyQualifiedName: "svc.raw.t2"},
		{ID: "id-3", FullyQualifiedName: "svc.raw.t3"},
	}
	previous := []omv1alpha1.TagAssignment{
		{EntityType: omv1alpha1.TaggableEntityTypeTable, EntityID: "id-2", FullyQualifiedName: "svc.raw.t2", TagFQN: "Tier.Tier3"},
		{EntityType: omv1alpha1.TaggableEntityTypeTable, EntityID: "id-4", FullyQualifiedName: "svc.raw.gone", TagFQN: "Tier.Tier3"},
	}

	add, rem := diffAssets(matched, previous, omv1alpha1.TaggableEntityTypeTable)

	sortByID := func(refs []omclient.AssetRef) {
		sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	}
	sortByID(add)
	sortByID(rem)

	wantAdd := []string{"id-1", "id-3"}
	wantRem := []string{"id-4"}

	if len(add) != len(wantAdd) {
		t.Fatalf("want %d adds, got %d (%+v)", len(wantAdd), len(add), add)
	}
	for i, ref := range add {
		if ref.ID != wantAdd[i] {
			t.Errorf("add[%d].ID = %q, want %q", i, ref.ID, wantAdd[i])
		}
		if ref.Type != string(omv1alpha1.TaggableEntityTypeTable) {
			t.Errorf("add[%d].Type = %q, want %q", i, ref.Type, omv1alpha1.TaggableEntityTypeTable)
		}
	}
	if len(rem) != len(wantRem) {
		t.Fatalf("want %d removes, got %d (%+v)", len(wantRem), len(rem), rem)
	}
	if rem[0].ID != wantRem[0] {
		t.Errorf("remove[0].ID = %q, want %q", rem[0].ID, wantRem[0])
	}
	if rem[0].FullyQualifiedName != "svc.raw.gone" {
		t.Errorf("remove[0].FQN = %q, want preserved from previous", rem[0].FullyQualifiedName)
	}
}

func TestDiffAssetsEmpty(t *testing.T) {
	add, rem := diffAssets(nil, nil, omv1alpha1.TaggableEntityTypeTable)
	if add != nil || rem != nil {
		t.Errorf("nil inputs should produce nil diffs; got add=%v rem=%v", add, rem)
	}
}

func TestRecordedTagFQN(t *testing.T) {
	tests := []struct {
		name        string
		assignments []omv1alpha1.TagAssignment
		want        string
	}{
		{name: "empty status returns empty string", assignments: nil, want: ""},
		{
			name: "single entry returns its tag",
			assignments: []omv1alpha1.TagAssignment{
				{TagFQN: "Tier.Tier3", EntityID: "1"},
			},
			want: "Tier.Tier3",
		},
		{
			name: "multiple entries return first (invariant: all share same tag)",
			assignments: []omv1alpha1.TagAssignment{
				{TagFQN: "Tier.Tier3", EntityID: "1"},
				{TagFQN: "Tier.Tier3", EntityID: "2"},
			},
			want: "Tier.Tier3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			et := &omv1alpha1.OpenMetadataEntityTag{
				Status: omv1alpha1.OpenMetadataEntityTagStatus{TagAssignments: tt.assignments},
			}
			if got := recordedTagFQN(et); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAssetRefsFromAssignments(t *testing.T) {
	in := []omv1alpha1.TagAssignment{
		{EntityType: omv1alpha1.TaggableEntityTypeTable, EntityID: "id-1", FullyQualifiedName: "svc.db.s.t1", TagFQN: "Tier.Tier3"},
		{EntityType: omv1alpha1.TaggableEntityTypeTable, EntityID: "id-2", FullyQualifiedName: "svc.db.s.t2", TagFQN: "Tier.Tier3"},
	}
	got := assetRefsFromAssignments(in)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "id-1" || got[0].Type != "table" || got[0].FullyQualifiedName != "svc.db.s.t1" {
		t.Errorf("unexpected first ref: %+v", got[0])
	}
}

func TestResolveEntitySearchIndex(t *testing.T) {
	tests := []struct {
		in   omv1alpha1.TaggableEntityType
		want string
	}{
		{omv1alpha1.TaggableEntityTypeTable, "table_search_index"},
		{omv1alpha1.TaggableEntityTypeTopic, "topic_search_index"},
		{omv1alpha1.TaggableEntityTypeDatabaseSchema, "database_schema_search_index"},
		{omv1alpha1.TaggableEntityTypeDatabase, "database_search_index"},
		{omv1alpha1.TaggableEntityTypeDashboard, "dashboard_search_index"},
		{omv1alpha1.TaggableEntityTypeMlmodel, "mlmodel_search_index"},
		{omv1alpha1.TaggableEntityTypePipeline, "pipeline_search_index"},
		{omv1alpha1.TaggableEntityTypeContainer, "container_search_index"},
		{omv1alpha1.TaggableEntityTypeSearchIndex, "search_entity_search_index"},
	}
	for _, tt := range tests {
		got, err := resolveEntitySearchIndex(tt.in)
		if err != nil {
			t.Errorf("resolveEntitySearchIndex(%q) returned error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("resolveEntitySearchIndex(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveEntitySearchIndexUnknown(t *testing.T) {
	_, err := resolveEntitySearchIndex(omv1alpha1.TaggableEntityType("nonexistent"))
	if err == nil {
		t.Fatal("expected error for unknown entity type")
	}
}
