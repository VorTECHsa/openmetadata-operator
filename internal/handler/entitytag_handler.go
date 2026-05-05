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

// EntityTagHandler contains the business logic for reconciling
// OpenMetadataEntityTag resources against the OpenMetadata API.
type EntityTagHandler struct {
	Client      client.Client
	Recorder    events.EventRecorder
	NewOMClient func(baseURL, token string) omclient.EntityTagClient
}

// Reconcile applies the desired tags to entities matched by the spec and
// reconciles any drift against status.appliedTags.
func (h *EntityTagHandler) Reconcile(ctx context.Context, et *omv1alpha1.OpenMetadataEntityTag) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// --- Observe ---

	searchIndex, err := resolveEntitySearchIndex(et.Spec.Match.EntityType)
	if err != nil {
		h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonUnsupportedEntityType, err.Error())
		return ctrl.Result{}, nil
	}

	// Resolve OpenMetadataConnection.
	conn := &omv1alpha1.OpenMetadataConnection{}
	if err := h.Client.Get(ctx, types.NamespacedName{Name: et.Spec.OpenMetadataConnectionRef}, conn); err != nil {
		logger.Error(err, "Failed to resolve OpenMetadataConnection", "ref", et.Spec.OpenMetadataConnectionRef)
		h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonConnectionNotFound, err.Error())
		return ctrl.Result{}, err
	}

	token, err := resolveAuthToken(ctx, h.Client, conn.Spec.AuthSecretRef)
	if err != nil {
		logger.Error(err, "Failed to resolve auth token")
		h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonAuthTokenUnavailable, err.Error())
		return ctrl.Result{}, err
	}

	omClient := h.NewOMClient(conn.Spec.URL, token)

	// Resolve FQN to UUID: the endpoints that attach/detach the tag to/from assets take the tag's UUID in the URL path.
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

	// Split status.TagAssignments into entries for the current desired tag (used to
	// compute the add/remove diff) and entries for any other tag (left over
	// from a previous spec — clean those up so renames take effect cleanly).
	previousForTag, staleByTag := splitAppliedByTag(et.Status.TagAssignments, tagFQN)

	toAdd, toRemove := diffAssets(matched, previousForTag, et.Spec.Match.EntityType)
	if len(toAdd) > 0 {
		if err := omClient.BulkAddTagToAssets(ctx, tagID, toAdd); err != nil {
			logger.Error(err, "Failed to bulk-add tag", "tagFQN", tagFQN)
			h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonTaggingFailed, err.Error())
			h.emitEvent(et, corev1.EventTypeWarning, omv1alpha1.ReasonTaggingFailed, err.Error())
			return ctrl.Result{}, err
		}
	}
	if len(toRemove) > 0 {
		if err := omClient.BulkRemoveTagFromAssets(ctx, tagID, toRemove); err != nil {
			logger.Error(err, "Failed to bulk-remove tag", "tagFQN", tagFQN)
			h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonTaggingFailed, err.Error())
			h.emitEvent(et, corev1.EventTypeWarning, omv1alpha1.ReasonTaggingFailed, err.Error())
			return ctrl.Result{}, err
		}
	}

	// Rename cleanup: remove any stale tag we previously applied under a
	// different FQN. 404 on the stale tag means it's already gone — fine.
	for staleFQN, applied := range staleByTag {
		staleID, err := omClient.GetEntityByName(ctx, tagsEntityTypePath, staleFQN)
		if err != nil {
			if omclient.IsNotFound(err) {
				continue
			}
			logger.Error(err, "Failed to resolve stale tag for cleanup", "tagFQN", staleFQN)
			h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonTagResolutionFailed, err.Error())
			return ctrl.Result{}, err
		}
		if err := omClient.BulkRemoveTagFromAssets(ctx, staleID, assetRefsFromApplied(applied)); err != nil {
			logger.Error(err, "Failed to bulk-remove stale tag", "tagFQN", staleFQN)
			h.setConditionAndPersist(ctx, et, metav1.ConditionFalse, omv1alpha1.ReasonTaggingFailed, err.Error())
			return ctrl.Result{}, err
		}
	}

	// New status: the desired tag on every matched entity.
	desiredApplied := make([]omv1alpha1.TagAssignment, 0, len(matched))
	for _, e := range matched {
		desiredApplied = append(desiredApplied, omv1alpha1.TagAssignment{
			EntityType:         et.Spec.Match.EntityType,
			EntityID:           e.ID,
			FullyQualifiedName: e.FullyQualifiedName,
			TagFQN:             tagFQN,
		})
	}
	sort.Slice(desiredApplied, func(i, j int) bool {
		return desiredApplied[i].FullyQualifiedName < desiredApplied[j].FullyQualifiedName
	})

	now := metav1.Now()
	et.Status.TagAssignments = desiredApplied
	et.Status.LastReconcileTime = &now
	et.Status.ObservedGeneration = et.Generation

	msg := fmt.Sprintf("Applied %s to %d %s entit(ies)", tagFQN, len(matched), et.Spec.Match.EntityType)
	h.setConditionAndPersist(ctx, et, metav1.ConditionTrue, omv1alpha1.ReasonInSync, msg)
	h.emitEvent(et, corev1.EventTypeNormal, omv1alpha1.ReasonInSync, msg)

	logger.Info("Reconciled OpenMetadataEntityTag",
		"entityType", et.Spec.Match.EntityType, "tagFQN", tagFQN, "matched", len(matched))

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// HandleDeletion removes our previously-applied tags from each entity recorded
// in status.appliedTags, then releases the finalizer. Iterates by TagFQN to
// support cleanup of historical specs (renames left lingering tags in status).
func (h *EntityTagHandler) HandleDeletion(ctx context.Context, et *omv1alpha1.OpenMetadataEntityTag) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	if !finalizer.IsPresent(et) {
		return ctrl.Result{}, nil
	}

	if len(et.Status.TagAssignments) > 0 {
		conn := &omv1alpha1.OpenMetadataConnection{}
		if err := h.Client.Get(ctx, types.NamespacedName{Name: et.Spec.OpenMetadataConnectionRef}, conn); err != nil {
			logger.Error(err, "Cannot resolve OpenMetadataConnection during deletion, retrying")
			return ctrl.Result{}, err
		}

		token, err := resolveAuthToken(ctx, h.Client, conn.Spec.AuthSecretRef)
		if err != nil {
			logger.Error(err, "Cannot resolve auth token during deletion, retrying")
			return ctrl.Result{}, err
		}

		omClient := h.NewOMClient(conn.Spec.URL, token)

		// Group applied tags by FQN (typically one bucket — only multiple if a
		// rename was in flight when the CR was deleted). Resolve each FQN to
		// its UUID and bulk-remove. 404s are treated as success (already gone).
		_, byTag := splitAppliedByTag(et.Status.TagAssignments, "")
		for tagFQN, applied := range byTag {
			tagID, err := omClient.GetEntityByName(ctx, tagsEntityTypePath, tagFQN)
			if err != nil {
				if omclient.IsNotFound(err) {
					continue
				}
				logger.Error(err, "Failed to resolve tag for deletion", "tagFQN", tagFQN)
				return ctrl.Result{}, err
			}
			if err := omClient.BulkRemoveTagFromAssets(ctx, tagID, assetRefsFromApplied(applied)); err != nil {
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

// splitAppliedByTag partitions status entries into those whose TagFQN matches
// `desired` (returned as a flat slice) and the rest grouped by their TagFQN.
// Pass an empty `desired` to fall through and group everything.
func splitAppliedByTag(applied []omv1alpha1.TagAssignment, desired string) ([]omv1alpha1.TagAssignment, map[string][]omv1alpha1.TagAssignment) {
	var current []omv1alpha1.TagAssignment
	other := make(map[string][]omv1alpha1.TagAssignment)
	for _, a := range applied {
		if a.TagFQN == desired {
			current = append(current, a)
		} else {
			other[a.TagFQN] = append(other[a.TagFQN], a)
		}
	}
	return current, other
}

// assetRefsFromApplied converts AppliedTag status entries into AssetRefs for
// the bulk tag-asset endpoint.
func assetRefsFromApplied(applied []omv1alpha1.TagAssignment) []omclient.AssetRef {
	refs := make([]omclient.AssetRef, 0, len(applied))
	for _, a := range applied {
		refs = append(refs, omclient.AssetRef{
			ID: a.EntityID, Type: string(a.EntityType), FullyQualifiedName: a.FullyQualifiedName,
		})
	}
	return refs
}

// diffAssets computes adds (entities matched now but not previously applied)
// and removes (entities previously applied but no longer matched). Identity is
// by entity ID.
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
