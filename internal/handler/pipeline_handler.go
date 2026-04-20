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
	"encoding/json"
	"errors"
	"fmt"

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

// pipelineAction describes the single reconciliation action to take.
type pipelineAction int

const (
	actionInSync       pipelineAction = iota // nothing to do
	actionCreate                             // new pipeline: upsert + deploy
	actionUpdate                             // spec changed: upsert + deploy
	actionCorrectDrift                       // remote version changed: upsert + deploy
	actionRedeploy                           // exists but not deployed: deploy only
)

// needsUpsert returns true if the action requires a PUT before deploy.
func (a pipelineAction) needsUpsert() bool {
	return a == actionCreate || a == actionUpdate || a == actionCorrectDrift
}

// conditionReason returns the appropriate status condition reason for the action.
func (a pipelineAction) conditionReason() string {
	switch a {
	case actionCreate:
		return omv1alpha1.ReasonCreated
	case actionCorrectDrift:
		return omv1alpha1.ReasonDriftCorrected
	case actionRedeploy:
		return omv1alpha1.ReasonDeployed
	default:
		return omv1alpha1.ReasonUpdated
	}
}

// decidePipelineAction determines what reconciliation action is needed by
// comparing the CR spec, observed generation, and remote state.
func decidePipelineAction(pipeline *omv1alpha1.IngestionPipeline, existing *omclient.PipelineResponse) pipelineAction {
	if existing == nil {
		return actionCreate
	}
	if pipeline.Generation != pipeline.Status.ObservedGeneration {
		return actionUpdate
	}
	if formatVersion(existing.Version) != pipeline.Status.RemoteVersion {
		return actionCorrectDrift
	}
	if !existing.Deployed {
		return actionRedeploy
	}
	return actionInSync
}

// entityTypeToPath maps EntityType values to their OpenMetadata API path segments.
var entityTypeToPath = map[omv1alpha1.EntityType]string{
	omv1alpha1.EntityTypeDatabaseService:  "services/databaseServices",
	omv1alpha1.EntityTypeMessagingService: "services/messagingServices",
	omv1alpha1.EntityTypeDashboardService: "services/dashboardServices",
	omv1alpha1.EntityTypePipelineService:  "services/pipelineServices",
	omv1alpha1.EntityTypeMlmodelService:   "services/mlmodelServices",
	omv1alpha1.EntityTypeStorageService:   "services/storageServices",
	omv1alpha1.EntityTypeSearchService:    "services/searchServices",
	omv1alpha1.EntityTypeMetadataService:  "services/metadataServices",
	omv1alpha1.EntityTypeAPIService:       "services/apiServices",
	omv1alpha1.EntityTypeTestSuite:        "dataQuality/testSuites",
}

// PipelineHandler contains the business logic for reconciling IngestionPipeline
// resources against the OpenMetadata API.
type PipelineHandler struct {
	// Client is the Kubernetes API client used to read secrets and update status.
	Client client.Client

	// Recorder emits Kubernetes events for significant lifecycle transitions.
	Recorder events.EventRecorder

	// NewOMClient constructs an OpenMetadata API client for pipeline operations.
	NewOMClient func(baseURL, token string) omclient.PipelineClient
}

// Reconcile executes the Observe -> Compare -> Converge loop for a single
// IngestionPipeline resource. It assumes the finalizer is already present.
func (h *PipelineHandler) Reconcile(ctx context.Context, pipeline *omv1alpha1.IngestionPipeline) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// --- Observe ---

	// Resolve OpenMetadataConnection reference.
	conn := &omv1alpha1.OpenMetadataConnection{}
	if err := h.Client.Get(ctx, types.NamespacedName{
		Name: pipeline.Spec.OpenMetadataConnectionRef,
	}, conn); err != nil {
		logger.Error(err, "Failed to resolve OpenMetadataConnection", "ref", pipeline.Spec.OpenMetadataConnectionRef)
		h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonConnectionNotFound, err.Error())
		return ctrl.Result{}, err
	}

	// Resolve OM auth token.
	token, err := resolveAuthToken(ctx, h.Client, conn.Spec.AuthSecretRef)
	if err != nil {
		logger.Error(err, "Failed to resolve auth token")
		h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonAuthTokenUnavailable, err.Error())
		return ctrl.Result{}, err
	}

	omClient := h.NewOMClient(conn.Spec.URL, token)

	// Resolve service entity FQN to OM UUID.
	servicePath, ok := entityTypeToPath[pipeline.Spec.ForOpenMetadata.Service.Type]
	if !ok {
		msg := fmt.Sprintf("unsupported service entity type %q", pipeline.Spec.ForOpenMetadata.Service.Type)
		h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonServiceNotFound, msg)
		return ctrl.Result{}, nil // Permanent error.
	}

	serviceID, err := omClient.GetEntityByName(ctx, servicePath, pipeline.Spec.ForOpenMetadata.Service.FullyQualifiedName)
	if err != nil {
		if omclient.IsNotFound(err) {
			msg := fmt.Sprintf("service %s %q not found in OpenMetadata", pipeline.Spec.ForOpenMetadata.Service.Type, pipeline.Spec.ForOpenMetadata.Service.FullyQualifiedName)
			logger.Info(msg)
			h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonServiceNotFound, msg)
			return ctrl.Result{}, err
		}
		logger.Error(err, "Failed to resolve service entity")
		h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonQueryFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Unmarshal sourceConfig.
	var sourceConfig map[string]any
	if err := json.Unmarshal(pipeline.Spec.ForOpenMetadata.SourceConfig.Raw, &sourceConfig); err != nil {
		h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonInvalidConfig,
			fmt.Sprintf("sourceConfig must be a key-value map: %v", err))
		return ctrl.Result{}, nil // Permanent error.
	}

	// --- Compare ---

	pipelineFQN := fmt.Sprintf("%s.%s", pipeline.Spec.ForOpenMetadata.Service.FullyQualifiedName, pipeline.Name)
	existing, err := omClient.GetPipelineByFQN(ctx, pipelineFQN)
	if err != nil && !omclient.IsNotFound(err) {
		logger.Error(err, "Failed to query pipeline from OpenMetadata")
		h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonQueryFailed, err.Error())
		return ctrl.Result{}, err
	}

	action := decidePipelineAction(pipeline, existing)

	if action == actionInSync {
		now := metav1.Now()
		pipeline.Status.LastReconcileTime = &now
		pipeline.Status.Deployed = true
		h.setConditionAndPersist(ctx, pipeline, metav1.ConditionTrue, omv1alpha1.ReasonInSync, "Pipeline is in sync with OpenMetadata")
		logger.V(1).Info("Pipeline is in sync, no upsert or deploy needed", "name", pipeline.Name)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// --- Converge ---

	if action.needsUpsert() {
		airflowConfig := buildAirflowConfig(pipeline.Spec.ForOpenMetadata.AirflowConfig)

		resolvedOwners, err := resolveOwners(ctx, omClient, pipeline.Spec.ForOpenMetadata.Owners)
		if err != nil {
			logger.Error(err, "Failed to resolve owners")
			h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonOwnerResolutionFailed, err.Error())
			if errors.Is(err, ErrUnsupportedOwnerType) {
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}

		pipelineReq := omclient.PipelineRequest{
			Name:          pipeline.Name,
			PipelineType:  string(pipeline.Spec.ForOpenMetadata.PipelineType),
			DisplayName:   pipeline.Spec.ForOpenMetadata.DisplayName,
			Description:   pipeline.Spec.ForOpenMetadata.Description,
			Owners:        resolvedOwners,
			SourceConfig:  sourceConfig,
			AirflowConfig: airflowConfig,
			Service: omclient.EntityRef{
				ID:   serviceID,
				Type: string(pipeline.Spec.ForOpenMetadata.Service.Type),
			},
		}

		resp, err := omClient.UpsertPipeline(ctx, pipelineReq)
		if err != nil {
			logger.Error(err, "Failed to upsert pipeline in OpenMetadata")
			h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonUpsertFailed, err.Error())
			h.emitEvent(pipeline, corev1.EventTypeWarning, omv1alpha1.ReasonUpsertFailed, err.Error())
			return ctrl.Result{}, err
		}

		pipeline.Status.OpenMetadataID = resp.ID
		pipeline.Status.FullyQualifiedName = resp.FullyQualifiedName
		pipeline.Status.ObservedGeneration = pipeline.Generation
		pipeline.Status.RemoteVersion = formatVersion(resp.Version)
	}

	// Deploy the pipeline to Airflow.
	if err := omClient.DeployPipeline(ctx, pipeline.Status.OpenMetadataID); err != nil {
		logger.Error(err, "Failed to deploy pipeline to Airflow")
		pipeline.Status.Deployed = false
		h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonDeployFailed, err.Error())
		h.emitEvent(pipeline, corev1.EventTypeWarning, omv1alpha1.ReasonDeployFailed, err.Error())
		return ctrl.Result{}, err
	}
	pipeline.Status.Deployed = true

	// Deploy increments the entity version but the POST /deploy endpoint only
	// returns an Airflow status (not the entity). Re-fetch to get the current
	// entity version so drift detection doesn't false-positive on the next cycle.
	postDeploy, err := omClient.GetPipelineByFQN(ctx, pipelineFQN)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("fetching post-deploy version: %w", err)
	}
	pipeline.Status.RemoteVersion = formatVersion(postDeploy.Version)

	now := metav1.Now()
	pipeline.Status.LastReconcileTime = &now

	reason := action.conditionReason()
	msg := fmt.Sprintf("Pipeline registered and deployed (ID: %s)", pipeline.Status.OpenMetadataID)
	h.setConditionAndPersist(ctx, pipeline, metav1.ConditionTrue, reason, msg)
	h.emitEvent(pipeline, corev1.EventTypeNormal, reason, msg)

	logger.Info("Successfully reconciled IngestionPipeline",
		"name", pipeline.Name, "openmetadataId", pipeline.Status.OpenMetadataID, "reason", reason)

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// HandleDeletion performs cleanup when the CR is being deleted.
func (h *PipelineHandler) HandleDeletion(ctx context.Context, pipeline *omv1alpha1.IngestionPipeline) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	if !finalizer.IsPresent(pipeline) {
		return ctrl.Result{}, nil
	}

	// Only call the OM API if we previously registered this pipeline.
	if pipeline.Status.OpenMetadataID != "" {
		conn := &omv1alpha1.OpenMetadataConnection{}
		if err := h.Client.Get(ctx, types.NamespacedName{
			Name: pipeline.Spec.OpenMetadataConnectionRef,
		}, conn); err != nil {
			logger.Error(err, "Cannot resolve OpenMetadataConnection during deletion, retrying")
			return ctrl.Result{}, err
		}

		token, err := resolveAuthToken(ctx, h.Client, conn.Spec.AuthSecretRef)
		if err != nil {
			logger.Error(err, "Cannot resolve auth token during deletion, retrying")
			return ctrl.Result{}, err
		}

		omClient := h.NewOMClient(conn.Spec.URL, token)
		if err := omClient.DeletePipeline(ctx, pipeline.Status.OpenMetadataID); err != nil {
			logger.Error(err, "Failed to delete pipeline from OpenMetadata, retrying")
			h.setConditionAndPersist(ctx, pipeline, metav1.ConditionFalse, omv1alpha1.ReasonDeletionFailed, err.Error())
			h.emitEvent(pipeline, corev1.EventTypeWarning, omv1alpha1.ReasonDeletionFailed, err.Error())
			return ctrl.Result{}, err
		}
		h.emitEvent(pipeline, corev1.EventTypeNormal, omv1alpha1.ReasonDeleted,
			fmt.Sprintf("Removed pipeline %s from OpenMetadata", pipeline.Status.OpenMetadataID))
		logger.Info("Deleted pipeline from OpenMetadata",
			"name", pipeline.Name, "openmetadataId", pipeline.Status.OpenMetadataID)
	}

	if err := finalizer.EnsureAbsent(ctx, h.Client, pipeline); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// buildAirflowConfig converts the CRD AirflowConfig to a map for the OM API payload.
func buildAirflowConfig(cfg omv1alpha1.AirflowConfig) map[string]any {
	m := map[string]any{}
	if cfg.ScheduleInterval != "" {
		m["scheduleInterval"] = cfg.ScheduleInterval
	}
	if cfg.StartDate != nil {
		m["startDate"] = cfg.StartDate.UTC().Format("2006-01-02T15:04:05.000000Z")
	}
	if cfg.Concurrency != 0 {
		m["concurrency"] = cfg.Concurrency
	}
	if cfg.Retries != 0 {
		m["retries"] = cfg.Retries
	}
	if cfg.RetryDelay != 0 {
		m["retryDelay"] = cfg.RetryDelay
	}
	if cfg.PausePipeline {
		m["pausePipeline"] = cfg.PausePipeline
	}
	if cfg.PipelineTimezone != "" {
		m["pipelineTimezone"] = cfg.PipelineTimezone
	}
	if cfg.PipelineCatchup {
		m["pipelineCatchup"] = cfg.PipelineCatchup
	}
	if cfg.MaxActiveRuns != 0 {
		m["maxActiveRuns"] = cfg.MaxActiveRuns
	}
	return m
}

// setConditionAndPersist updates the Ready condition on the CR and writes the
// status sub-resource.
func (h *PipelineHandler) setConditionAndPersist(ctx context.Context, pipeline *omv1alpha1.IngestionPipeline,
	status metav1.ConditionStatus, reason, message string) {

	condition.SetReady(&pipeline.Status.Conditions, pipeline.Generation, status, reason, message)

	if err := h.Client.Status().Update(ctx, pipeline); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status condition")
	}
}

// emitEvent records a Kubernetes event if a Recorder is configured.
func (h *PipelineHandler) emitEvent(obj *omv1alpha1.IngestionPipeline, eventType, reason, message string) {
	if h.Recorder != nil {
		h.Recorder.Eventf(obj, nil, eventType, reason, "Reconcile", message)
	}
}
