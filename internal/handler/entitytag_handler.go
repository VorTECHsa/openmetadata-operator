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
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/condition"
	"github.com/VorTECHsa/openmetadata-operator/internal/finalizer"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

// tagsEntityTypePath is the OpenMetadata entity type path used to look up
// a tag by FQN (GET /api/v1/tags/name/{fqn}).
const tagsEntityTypePath = "tags"

// EntityTagHandler reconciles OpenMetadataEntityTag resources against
// the OpenMetadata API.
type EntityTagHandler struct {
	Client      client.Client
	Recorder    events.EventRecorder
	NewOMClient func(baseURL, token string) omclient.EntityTagClient
}

// Reconcile follows Observe → Compare → Converge:
//   - Observe: validate inputs, resolve the OM connection, look up the tag's UUID.
//   - Compare: search OM for entities matching the spec's includes/excludes.
//   - Converge: pick one of two paths based on what status records:
//     1) Steady state — tag in status is the same as in spec, so we just
//     add the tag to matched entities that don't have it yet and remove it
//     from entities that the spec no longer matches.
//     2) Rename — tag in status differs from spec, so we add the new tag
//     to every matched entity and remove the old tag from every entity
//     listed in status.
//
// Status invariant: after every successful reconcile, every entry in
// status.TagAssignments shares the same tagFQN — the one written this run.
// We use that to detect renames cheaply: if the tagFQN recorded in status
// differs from the spec's, the user has changed the desired tag.
func (h *EntityTagHandler) Reconcile(ctx context.Context, et *omv1alpha1.OpenMetadataEntityTag) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// --- Observe ---

	searchIndex, err := resolveEntitySearchIndex(et.Spec.Match.EntityType)
	if err != nil {
		h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonUnsupportedEntityType, err.Error())
		return ctrl.Result{}, nil
	}

	omClient, err := h.resolveOMClient(ctx, et)
	if err != nil {
		return ctrl.Result{}, err
	}

	tagFQN := et.Spec.Tag.TagFQN
	tagID, err := omClient.GetEntityByName(ctx, tagsEntityTypePath, tagFQN)
	if err != nil {
		if omclient.IsNotFound(err) {
			h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonTagResolutionFailed,
				fmt.Sprintf("tag not found: %q", tagFQN))
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to resolve tag", "tagFQN", tagFQN)
		h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonTagResolutionFailed, err.Error())
		return ctrl.Result{}, err
	}

	// --- Compare ---

	matched, err := omClient.SearchEntities(ctx, searchIndex, et.Spec.Match.Includes, et.Spec.Match.Excludes)
	if err != nil {
		logger.Error(err, "Failed to search entities", "entityType", et.Spec.Match.EntityType, "searchIndex", searchIndex)
		h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonEntitySearchFailed, err.Error())
		return ctrl.Result{}, err
	}

	// --- Converge ---

	// Rename path triggers only when status records a tag (status non-empty)
	// and that recorded tag differs from what the spec now wants.
	if oldTagFQN := recordedTagFQN(et); oldTagFQN != "" && oldTagFQN != tagFQN {
		if err := h.applyRename(ctx, omClient, et, tagID, oldTagFQN, matched); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if err := h.applyDiff(ctx, omClient, et, tagID, matched); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Persist new status: the desired tag on every matched entity.
	et.Status.TagAssignments = buildAssignments(matched, et.Spec.Match.EntityType, tagFQN)
	now := metav1.Now()
	et.Status.LastReconcileTime = &now
	et.Status.ObservedGeneration = et.Generation

	msg := fmt.Sprintf("Applied %s to %d %s entities", tagFQN, len(matched), et.Spec.Match.EntityType)
	h.setConditionAndPersist(ctx, et, metav1.ConditionTrue, omv1alpha1.ReasonInSync, msg)
	h.emitEvent(et, corev1.EventTypeNormal, omv1alpha1.ReasonInSync, msg)

	logger.Info("Reconciled OpenMetadataEntityTag",
		"entityType", et.Spec.Match.EntityType, "tagFQN", tagFQN, "matched", len(matched))

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// applyDiff is the steady-state path: status.TagAssignments either records the
// same tagFQN as the spec (or is empty on first reconcile). Diff matched
// entities against what we previously applied — bulk-add newcomers, bulk-remove
// the ones that fell out of scope.
func (h *EntityTagHandler) applyDiff(ctx context.Context, omClient omclient.EntityTagClient, et *omv1alpha1.OpenMetadataEntityTag, tagID string, matched []omclient.EntitySummary) error {
	toAdd, toRemove := diffAssets(matched, et.Status.TagAssignments, et.Spec.Match.EntityType)

	if len(toAdd) > 0 {
		if err := omClient.BulkAddTagToAssets(ctx, tagID, toAdd); err != nil {
			return h.failTagging(ctx, et, "Failed to bulk-add tag", err)
		}
	}
	if len(toRemove) > 0 {
		if err := omClient.BulkRemoveTagFromAssets(ctx, tagID, toRemove); err != nil {
			return h.failTagging(ctx, et, "Failed to bulk-remove tag", err)
		}
	}
	return nil
}

// applyRename is the rename path: status.TagAssignments records assignments
// under oldTagFQN, but the spec now wants tagFQN. Apply the new tag to every
// matched entity (no diff needed — by invariant none of them carry it yet
// from this CR), then remove the old tag from every entity we previously
// applied it to. 404 on the old tag's lookup means it's already gone in OM
// — fine, nothing to remove.
func (h *EntityTagHandler) applyRename(ctx context.Context, omClient omclient.EntityTagClient, et *omv1alpha1.OpenMetadataEntityTag, tagID, oldTagFQN string, matched []omclient.EntitySummary) error {
	if newRefs := assetRefsFromMatched(matched, et.Spec.Match.EntityType); len(newRefs) > 0 {
		if err := omClient.BulkAddTagToAssets(ctx, tagID, newRefs); err != nil {
			return h.failTagging(ctx, et, "Failed to bulk-add tag during rename", err)
		}
	}

	oldTagID, err := omClient.GetEntityByName(ctx, tagsEntityTypePath, oldTagFQN)
	if err != nil {
		if omclient.IsNotFound(err) {
			return nil
		}
		logf.FromContext(ctx).Error(err, "Failed to resolve old tag for rename cleanup", "tagFQN", oldTagFQN)
		h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonTagResolutionFailed, err.Error())
		return err
	}
	oldRefs := assetRefsFromAssignments(et.Status.TagAssignments)
	if err := omClient.BulkRemoveTagFromAssets(ctx, oldTagID, oldRefs); err != nil {
		return h.failTagging(ctx, et, "Failed to bulk-remove old tag during rename", err)
	}
	return nil
}

// HandleDeletion removes our previously-applied tags from each recorded asset,
// then releases the finalizer. Iterates by tagFQN so the rare case of a CR
// being deleted mid-rename (status holds entries under more than one FQN) is
// handled correctly.
func (h *EntityTagHandler) HandleDeletion(ctx context.Context, et *omv1alpha1.OpenMetadataEntityTag) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	if !finalizer.IsPresent(et) {
		return ctrl.Result{}, nil
	}

	if len(et.Status.TagAssignments) > 0 {
		omClient, err := h.resolveOMClient(ctx, et)
		if err != nil {
			return ctrl.Result{}, err
		}

		tagFQN := recordedTagFQN(et)
		tagID, err := omClient.GetEntityByName(ctx, tagsEntityTypePath, tagFQN)
		if err != nil && !omclient.IsNotFound(err) {
			logger.Error(err, "Failed to resolve tag for deletion", "tagFQN", tagFQN)
			return ctrl.Result{}, err
		}
		// On 404 the tag is already gone in OM, so there's nothing to remove.
		if err == nil {
			if err := omClient.BulkRemoveTagFromAssets(ctx, tagID, assetRefsFromAssignments(et.Status.TagAssignments)); err != nil {
				logger.Error(err, "Failed to bulk-remove tag during deletion", "tagFQN", tagFQN)
				h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonTaggingFailed, err.Error())
				return ctrl.Result{}, err
			}
		}
	}

	if err := finalizer.EnsureAbsent(ctx, h.Client, et); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// resolveOMClient resolves the OpenMetadataConnection ref and auth token for
// the given CR, returning a configured client. Sets the appropriate condition
// and persists status on failure.
func (h *EntityTagHandler) resolveOMClient(ctx context.Context, et *omv1alpha1.OpenMetadataEntityTag) (omclient.EntityTagClient, error) {
	logger := logf.FromContext(ctx)

	conn := &omv1alpha1.OpenMetadataConnection{}
	if err := h.Client.Get(ctx, types.NamespacedName{Name: et.Spec.OpenMetadataConnectionRef}, conn); err != nil {
		logger.Error(err, "Failed to resolve OpenMetadataConnection", "ref", et.Spec.OpenMetadataConnectionRef)
		h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonConnectionNotFound, err.Error())
		return nil, err
	}

	token, err := resolveAuthToken(ctx, h.Client, conn.Spec.AuthSecretRef)
	if err != nil {
		logger.Error(err, "Failed to resolve auth token")
		h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonAuthTokenUnavailable, err.Error())
		return nil, err
	}

	return h.NewOMClient(conn.Spec.URL, token), nil
}

// When a bulk tag-asset call fails.
func (h *EntityTagHandler) failTagging(ctx context.Context, et *omv1alpha1.OpenMetadataEntityTag, msg string, err error) error {
	logf.FromContext(ctx).Error(err, msg)
	h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonTaggingFailed, err.Error())
	h.emitEvent(et, corev1.EventTypeWarning, omv1alpha1.ReasonTaggingFailed, err.Error())
	return err
}

// recordedTagFQN returns the tagFQN recorded in status, or "" if status is
// empty (first reconcile). Relies on the status invariant that every entry
// shares the same tagFQN, so the first entry is representative.
func recordedTagFQN(et *omv1alpha1.OpenMetadataEntityTag) string {
	if len(et.Status.TagAssignments) == 0 {
		return ""
	}
	return et.Status.TagAssignments[0].TagFQN
}

// buildAssignments returns one TagAssignment per matched entity, sorted by
// FQN for stable status output.
func buildAssignments(matched []omclient.EntitySummary, entityType omv1alpha1.TaggableEntityType, tagFQN string) []omv1alpha1.TagAssignment {
	out := make([]omv1alpha1.TagAssignment, 0, len(matched))
	for _, e := range matched {
		out = append(out, omv1alpha1.TagAssignment{
			EntityType:         entityType,
			EntityID:           e.ID,
			FullyQualifiedName: e.FullyQualifiedName,
			TagFQN:             tagFQN,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FullyQualifiedName < out[j].FullyQualifiedName
	})
	return out
}

// assetRefsFromMatched converts search results into AssetRefs for the bulk
// tag-asset endpoint.
func assetRefsFromMatched(matched []omclient.EntitySummary, entityType omv1alpha1.TaggableEntityType) []omclient.AssetRef {
	refs := make([]omclient.AssetRef, 0, len(matched))
	for _, e := range matched {
		refs = append(refs, omclient.AssetRef{
			ID: e.ID, Type: string(entityType), FullyQualifiedName: e.FullyQualifiedName,
		})
	}
	return refs
}

// assetRefsFromAssignments converts status.TagAssignments entries into
// AssetRefs for the bulk tag-asset endpoint.
func assetRefsFromAssignments(assignments []omv1alpha1.TagAssignment) []omclient.AssetRef {
	refs := make([]omclient.AssetRef, 0, len(assignments))
	for _, a := range assignments {
		refs = append(refs, omclient.AssetRef{
			ID: a.EntityID, Type: string(a.EntityType), FullyQualifiedName: a.FullyQualifiedName,
		})
	}
	return refs
}

// diffAssets computes adds (in matched but not previously applied) and
// removes (previously applied but no longer matched). Identity is by
// entity ID.
func diffAssets(matched []omclient.EntitySummary, previous []omv1alpha1.TagAssignment, entityType omv1alpha1.TaggableEntityType) (toAdd, toRemove []omclient.AssetRef) {
	matchedByID := make(map[string]omclient.EntitySummary, len(matched))
	for _, e := range matched {
		matchedByID[e.ID] = e
	}
	prevByID := make(map[string]omv1alpha1.TagAssignment, len(previous))
	for _, p := range previous {
		prevByID[p.EntityID] = p
	}

	for id, e := range matchedByID {
		if _, ok := prevByID[id]; !ok {
			toAdd = append(toAdd, omclient.AssetRef{
				ID: id, Type: string(entityType), FullyQualifiedName: e.FullyQualifiedName,
			})
		}
	}
	for id, p := range prevByID {
		if _, ok := matchedByID[id]; !ok {
			toRemove = append(toRemove, omclient.AssetRef{
				ID: id, Type: string(p.EntityType), FullyQualifiedName: p.FullyQualifiedName,
			})
		}
	}
	return toAdd, toRemove
}

func (h *EntityTagHandler) setConditionAndPersist(ctx context.Context, et *omv1alpha1.OpenMetadataEntityTag,
	status metav1.ConditionStatus, reason, message string) {

	condition.SetReady(&et.Status.Conditions, et.Generation, status, reason, message)

	if err := h.Client.Status().Update(ctx, et); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status condition")
	}
}

func (h *EntityTagHandler) emitEvent(obj *omv1alpha1.OpenMetadataEntityTag, eventType, reason, message string) {
	if h.Recorder != nil {
		h.Recorder.Eventf(obj, nil, eventType, reason, "Reconcile", message)
	}
}
