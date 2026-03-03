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

// TestCaseHandler contains the business logic for reconciling OpenMetadataTestCase
// resources against the OpenMetadata API.
type TestCaseHandler struct {
	// Client is the Kubernetes API client used to read secrets and update status.
	Client client.Client

	// Recorder emits Kubernetes events for significant lifecycle transitions.
	Recorder events.EventRecorder

	// NewOMClient constructs an OpenMetadata API client for test case operations.
	NewOMClient func(baseURL, token string) omclient.TestCaseClient
}

// Reconcile executes the Observe -> Compare -> Converge loop for a single
// OpenMetadataTestCase resource. It assumes the finalizer is already present.
func (h *TestCaseHandler) Reconcile(ctx context.Context, tc *omv1alpha1.OpenMetadataTestCase) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// --- Observe ---

	// Resolve OpenMetadataConnection reference.
	conn := &omv1alpha1.OpenMetadataConnection{}
	if err := h.Client.Get(ctx, types.NamespacedName{
		Name: tc.Spec.OpenMetadataConnectionRef,
	}, conn); err != nil {
		logger.Error(err, "Failed to resolve OpenMetadataConnection", "ref", tc.Spec.OpenMetadataConnectionRef)
		h.setConditionAndPersist(ctx, tc, metav1.ConditionFalse, omv1alpha1.ReasonConnectionNotFound, err.Error())
		return ctrl.Result{}, err
	}

	// Resolve OM auth token.
	token, err := resolveAuthToken(ctx, h.Client, conn.Spec.AuthSecretRef)
	if err != nil {
		logger.Error(err, "Failed to resolve auth token")
		h.setConditionAndPersist(ctx, tc, metav1.ConditionFalse, omv1alpha1.ReasonAuthTokenUnavailable, err.Error())
		return ctrl.Result{}, err
	}

	omClient := h.NewOMClient(conn.Spec.URL, token)

	// --- Compare ---

	existing, err := omClient.GetTestCaseByFQN(ctx, tc.Name)
	notFound := omclient.IsNotFound(err)
	if err != nil && !notFound {
		logger.Error(err, "Failed to query test case from OpenMetadata")
		h.setConditionAndPersist(ctx, tc, metav1.ConditionFalse, omv1alpha1.ReasonQueryFailed, err.Error())
		return ctrl.Result{}, err
	}

	needsCreate := notFound
	specChanged := tc.Generation != tc.Status.ObservedGeneration
	remoteVersionStr := ""
	if !needsCreate {
		remoteVersionStr = formatVersion(existing.Version)
	}
	remoteDrifted := !needsCreate && remoteVersionStr != tc.Status.RemoteVersion

	if !needsCreate && !specChanged && !remoteDrifted {
		now := metav1.Now()
		tc.Status.LastReconcileTime = &now
		h.setConditionAndPersist(ctx, tc, metav1.ConditionTrue, omv1alpha1.ReasonInSync, "Test case is in sync with OpenMetadata")
		logger.V(1).Info("Test case is in sync, no upsert needed", "name", tc.Name)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// --- Converge ---

	tcReq := omclient.TestCaseRequest{
		Name:                        tc.Name,
		TestDefinition:              tc.Spec.ForOpenMetadata.TestDefinition,
		EntityLink:                  tc.Spec.ForOpenMetadata.EntityLink,
		DisplayName:                 tc.Spec.ForOpenMetadata.DisplayName,
		Description:                 tc.Spec.ForOpenMetadata.Description,
		ComputePassedFailedRowCount: tc.Spec.ForOpenMetadata.ComputePassedFailedRowCount,
	}

	// Map parameter values from CRD type to omclient type.
	for _, pv := range tc.Spec.ForOpenMetadata.ParameterValues {
		tcReq.ParameterValues = append(tcReq.ParameterValues, omclient.TestCaseParameterValue{
			Name:  pv.Name,
			Value: pv.Value,
		})
	}

	resp, err := omClient.UpsertTestCase(ctx, tcReq)
	if err != nil {
		logger.Error(err, "Failed to upsert test case in OpenMetadata")
		h.setConditionAndPersist(ctx, tc, metav1.ConditionFalse, omv1alpha1.ReasonUpsertFailed, err.Error())
		h.emitEvent(tc, corev1.EventTypeWarning, omv1alpha1.ReasonUpsertFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Update status with the response from OpenMetadata.
	now := metav1.Now()
	tc.Status.OpenMetadataID = resp.ID
	tc.Status.FullyQualifiedName = resp.FullyQualifiedName
	tc.Status.TestSuiteID = resp.TestSuite.ID
	tc.Status.LastReconcileTime = &now
	tc.Status.ObservedGeneration = tc.Generation
	tc.Status.RemoteVersion = formatVersion(resp.Version)

	reason := omv1alpha1.ReasonUpdated
	if needsCreate {
		reason = omv1alpha1.ReasonCreated
	} else if remoteDrifted {
		reason = omv1alpha1.ReasonDriftCorrected
	}
	msg := fmt.Sprintf("Test case registered (ID: %s)", resp.ID)
	h.setConditionAndPersist(ctx, tc, metav1.ConditionTrue, reason, msg)
	h.emitEvent(tc, corev1.EventTypeNormal, reason, msg)

	logger.Info("Successfully reconciled OpenMetadataTestCase",
		"name", tc.Name, "openmetadataId", resp.ID, "reason", reason)

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// HandleDeletion performs cleanup when the CR is being deleted.
func (h *TestCaseHandler) HandleDeletion(ctx context.Context, tc *omv1alpha1.OpenMetadataTestCase) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	if !finalizer.IsPresent(tc) {
		return ctrl.Result{}, nil
	}

	// Only call the OM API if we previously registered this test case.
	if tc.Status.OpenMetadataID != "" {
		conn := &omv1alpha1.OpenMetadataConnection{}
		if err := h.Client.Get(ctx, types.NamespacedName{
			Name: tc.Spec.OpenMetadataConnectionRef,
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
		if err := omClient.DeleteTestCase(ctx, tc.Status.OpenMetadataID); err != nil {
			logger.Error(err, "Failed to delete test case from OpenMetadata, retrying")
			h.setConditionAndPersist(ctx, tc, metav1.ConditionFalse, omv1alpha1.ReasonDeletionFailed, err.Error())
			h.emitEvent(tc, corev1.EventTypeWarning, omv1alpha1.ReasonDeletionFailed, err.Error())
			return ctrl.Result{}, err
		}
		h.emitEvent(tc, corev1.EventTypeNormal, omv1alpha1.ReasonDeleted,
			fmt.Sprintf("Removed test case %s from OpenMetadata", tc.Status.OpenMetadataID))
		logger.Info("Deleted test case from OpenMetadata",
			"name", tc.Name, "openmetadataId", tc.Status.OpenMetadataID)
	}

	if err := finalizer.EnsureAbsent(ctx, h.Client, tc); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// setConditionAndPersist updates the Ready condition on the CR and writes the
// status sub-resource.
func (h *TestCaseHandler) setConditionAndPersist(ctx context.Context, tc *omv1alpha1.OpenMetadataTestCase,
	status metav1.ConditionStatus, reason, message string) {

	condition.SetReady(&tc.Status.Conditions, tc.Generation, status, reason, message)

	if err := h.Client.Status().Update(ctx, tc); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status condition")
	}
}

// emitEvent records a Kubernetes event if a Recorder is configured.
func (h *TestCaseHandler) emitEvent(obj *omv1alpha1.OpenMetadataTestCase, eventType, reason, message string) {
	if h.Recorder != nil {
		h.Recorder.Eventf(obj, nil, eventType, reason, "Reconcile", message)
	}
}
