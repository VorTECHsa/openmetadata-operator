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

// Package handler implements the business logic for reconciling OpenMetadata
// custom resources. It follows the Observe -> Compare -> Converge pattern,
// keeping the controller thin and focused on orchestration.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

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
	"github.com/VorTECHsa/openmetadata-operator/internal/secretresolver"
)

// requeueInterval is the default interval between periodic reconciliations
// when the resource is in sync (drift detection).
const requeueInterval = 5 * time.Minute

// formatVersion converts an OpenMetadata entity version (float64) to its
// string representation for storage in status.remoteVersion.
func formatVersion(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ServiceHandler contains the business logic for reconciling OpenMetadataService
// resources against the OpenMetadata API. It is injected into the controller.
type ServiceHandler struct {
	// Client is the Kubernetes API client used to read secrets and update status.
	Client client.Client

	// Recorder emits Kubernetes events for significant lifecycle transitions.
	Recorder events.EventRecorder

	// NewOMClient constructs an OpenMetadata API client for a given base URL
	// and auth token. Must be set before calling Reconcile or HandleDeletion.
	NewOMClient func(baseURL, token string) omclient.ServiceClient
}

// Reconcile executes the Observe -> Compare -> Converge loop for a single
// OpenMetadataService resource. It assumes the finalizer is already present.
func (h *ServiceHandler) Reconcile(ctx context.Context, svc *omv1alpha1.OpenMetadataService) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// --- Observe ---

	// Validate service type.
	endpoint, err := endpointForServiceType(svc.Spec.ForOpenMetadata.ServiceType)
	if err != nil {
		h.setConditionAndPersist(ctx, svc, metav1.ConditionFalse, omv1alpha1.ReasonUnsupportedServiceType, err.Error())
		return ctrl.Result{}, nil // Permanent error — do not requeue.
	}

	// Resolve OpenMetadataConnection reference.
	conn := &omv1alpha1.OpenMetadataConnection{}
	if err := h.Client.Get(ctx, types.NamespacedName{
		Name: svc.Spec.OpenMetadataConnectionRef,
	}, conn); err != nil {
		logger.Error(err, "Failed to resolve OpenMetadataConnection", "ref", svc.Spec.OpenMetadataConnectionRef)
		h.setConditionAndPersist(ctx, svc, metav1.ConditionFalse, omv1alpha1.ReasonConnectionNotFound, err.Error())
		return ctrl.Result{}, err
	}

	// Resolve OM auth token.
	token, err := resolveAuthToken(ctx, h.Client, conn.Spec.AuthSecretRef)
	if err != nil {
		logger.Error(err, "Failed to resolve auth token")
		h.setConditionAndPersist(ctx, svc, metav1.ConditionFalse, omv1alpha1.ReasonAuthTokenUnavailable, err.Error())
		return ctrl.Result{}, err
	}

	omClient := h.NewOMClient(conn.Spec.URL, token)

	// Unmarshal connection config.
	var rawConfig map[string]any
	if err := json.Unmarshal(svc.Spec.ForOpenMetadata.Connection.Config.Raw, &rawConfig); err != nil {
		h.setConditionAndPersist(ctx, svc, metav1.ConditionFalse, omv1alpha1.ReasonInvalidConfig,
			fmt.Sprintf("connection config must be a key-value map: %v", err))
		return ctrl.Result{}, nil // Permanent error.
	}

	// Resolve secret references in the connection config.
	resolver := secretresolver.NewResolver(h.Client, svc.Namespace)
	resolvedConfig, err := resolver.Resolve(ctx, rawConfig)
	if err != nil {
		logger.Error(err, "Failed to resolve secret references")
		h.setConditionAndPersist(ctx, svc, metav1.ConditionFalse, omv1alpha1.ReasonSecretResolutionFailed, err.Error())
		return ctrl.Result{}, err
	}

	// --- Compare ---

	existing, err := omClient.GetServiceByName(ctx, endpoint, svc.Name)
	notFound := omclient.IsNotFound(err)
	if err != nil && !notFound {
		logger.Error(err, "Failed to query service from OpenMetadata")
		h.setConditionAndPersist(ctx, svc, metav1.ConditionFalse, omv1alpha1.ReasonQueryFailed, err.Error())
		return ctrl.Result{}, err
	}

	needsCreate := notFound
	specChanged := svc.Generation != svc.Status.ObservedGeneration
	remoteVersionStr := ""
	if !needsCreate {
		remoteVersionStr = formatVersion(existing.Version)
	}
	remoteDrifted := !needsCreate && remoteVersionStr != svc.Status.RemoteVersion

	// If the service exists, spec has not changed, and remote has not drifted,
	// there is nothing to do — desired state matches actual state.
	if !needsCreate && !specChanged && !remoteDrifted {
		now := metav1.Now()
		svc.Status.LastReconcileTime = &now
		h.setConditionAndPersist(ctx, svc, metav1.ConditionTrue, omv1alpha1.ReasonInSync, "Service is in sync with OpenMetadata")
		logger.V(1).Info("Service is in sync, no upsert needed", "name", svc.Name)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// --- Converge ---

	serviceReq := omclient.ServiceRequest{
		Name:        svc.Name,
		ServiceType: string(svc.Spec.ForOpenMetadata.ServiceType),
		DisplayName: svc.Spec.ForOpenMetadata.DisplayName,
		Description: svc.Spec.ForOpenMetadata.Description,
		Connection:  map[string]any{"config": resolvedConfig},
	}

	resp, err := omClient.UpsertService(ctx, endpoint, serviceReq)
	if err != nil {
		logger.Error(err, "Failed to upsert service in OpenMetadata")
		h.setConditionAndPersist(ctx, svc, metav1.ConditionFalse, omv1alpha1.ReasonUpsertFailed, err.Error())
		h.emitEvent(svc, corev1.EventTypeWarning, omv1alpha1.ReasonUpsertFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Update status with the response from OpenMetadata.
	now := metav1.Now()
	svc.Status.OpenMetadataID = resp.ID
	svc.Status.FullyQualifiedName = resp.FullyQualifiedName
	svc.Status.LastReconcileTime = &now
	svc.Status.ObservedGeneration = svc.Generation
	svc.Status.RemoteVersion = formatVersion(resp.Version)

	reason := omv1alpha1.ReasonUpdated
	if needsCreate {
		reason = omv1alpha1.ReasonCreated
	} else if remoteDrifted {
		reason = omv1alpha1.ReasonDriftCorrected
	}
	msg := fmt.Sprintf("Service registered (ID: %s)", resp.ID)
	h.setConditionAndPersist(ctx, svc, metav1.ConditionTrue, reason, msg)
	h.emitEvent(svc, corev1.EventTypeNormal, reason, msg)

	logger.Info("Successfully reconciled OpenMetadataService",
		"name", svc.Name, "openmetadataId", resp.ID, "reason", reason)

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// HandleDeletion performs cleanup when the CR is being deleted. It deletes the
// corresponding service from OpenMetadata and removes the finalizer.
func (h *ServiceHandler) HandleDeletion(ctx context.Context, svc *omv1alpha1.OpenMetadataService) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	if !finalizer.IsPresent(svc) {
		return ctrl.Result{}, nil
	}

	// Only call the OM API if we previously registered this service.
	if svc.Status.OpenMetadataID != "" {
		conn := &omv1alpha1.OpenMetadataConnection{}
		if err := h.Client.Get(ctx, types.NamespacedName{
			Name: svc.Spec.OpenMetadataConnectionRef,
		}, conn); err != nil {
			logger.Error(err, "Cannot resolve OpenMetadataConnection during deletion, retrying")
			return ctrl.Result{}, err
		}

		token, err := resolveAuthToken(ctx, h.Client, conn.Spec.AuthSecretRef)
		if err != nil {
			logger.Error(err, "Cannot resolve auth token during deletion, retrying")
			return ctrl.Result{}, err
		}

		endpoint, err := endpointForServiceType(svc.Spec.ForOpenMetadata.ServiceType)
		if err != nil {
			logger.Error(err, "Unknown service type during deletion, removing finaliser anyway")
		} else {
			omClient := h.NewOMClient(conn.Spec.URL, token)
			if err := omClient.DeleteService(ctx, endpoint, svc.Status.OpenMetadataID); err != nil {
				logger.Error(err, "Failed to delete service from OpenMetadata, retrying")
				h.setConditionAndPersist(ctx, svc, metav1.ConditionFalse, omv1alpha1.ReasonDeletionFailed, err.Error())
				h.emitEvent(svc, corev1.EventTypeWarning, omv1alpha1.ReasonDeletionFailed, err.Error())
				return ctrl.Result{}, err
			}
			h.emitEvent(svc, corev1.EventTypeNormal, omv1alpha1.ReasonDeleted,
				fmt.Sprintf("Removed service %s from OpenMetadata", svc.Status.OpenMetadataID))
			logger.Info("Deleted service from OpenMetadata",
				"name", svc.Name, "openmetadataId", svc.Status.OpenMetadataID)
		}
	}

	if err := finalizer.EnsureAbsent(ctx, h.Client, svc); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// setConditionAndPersist updates the Ready condition on the CR and writes the
// status sub-resource. Errors are logged but not propagated to the caller —
// the reconciler relies on the next requeue to retry.
func (h *ServiceHandler) setConditionAndPersist(ctx context.Context, svc *omv1alpha1.OpenMetadataService,
	status metav1.ConditionStatus, reason, message string) {

	condition.SetReady(&svc.Status.Conditions, svc.Generation, status, reason, message)

	if err := h.Client.Status().Update(ctx, svc); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status condition")
	}
}

// emitEvent records a Kubernetes event if a Recorder is configured.
func (h *ServiceHandler) emitEvent(obj *omv1alpha1.OpenMetadataService, eventType, reason, message string) {
	if h.Recorder != nil {
		h.Recorder.Eventf(obj, nil, eventType, reason, "Reconcile", message)
	}
}
