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

package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
	"github.com/VorTECHsa/openmetadata-operator/internal/finalizer"
	"github.com/VorTECHsa/openmetadata-operator/internal/handler"
	"github.com/VorTECHsa/openmetadata-operator/internal/omclient"
)

// OpenMetadataServiceReconciler reconciles an OpenMetadataService object.
// It acts as a thin orchestrator: fetching the CR, managing the finalizer
// lifecycle, and delegating all business logic to the ServiceHandler.
type OpenMetadataServiceReconciler struct {
	client.Client
	Handler *handler.ServiceHandler
}

// +kubebuilder:rbac:groups=openmetadata.vortexa.com,resources=openmetadataservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openmetadata.vortexa.com,resources=openmetadataservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openmetadata.vortexa.com,resources=openmetadataservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=openmetadata.vortexa.com,resources=openmetadataconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles a single reconciliation loop for an OpenMetadataService resource.
func (r *OpenMetadataServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	svc := &omv1alpha1.OpenMetadataService{}
	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Delegate deletion to the handler (which also removes the finalizer).
	if !svc.DeletionTimestamp.IsZero() {
		return r.Handler.HandleDeletion(ctx, svc)
	}

	// Ensure the finalizer is present before any business logic runs.
	added, err := finalizer.EnsurePresent(ctx, r.Client, svc)
	if err != nil {
		return ctrl.Result{}, err
	}
	if added {
		return ctrl.Result{}, nil
	}

	// Delegate the observe/compare/converge loop to the handler.
	return r.Handler.Reconcile(ctx, svc)
}

// SetupWithManager sets up the controller with the Manager. If Handler is nil,
// a default ServiceHandler is wired with real dependencies. Tests can pre-set
// Handler to substitute mock implementations.
func (r *OpenMetadataServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Handler == nil {
		r.Handler = &handler.ServiceHandler{
			Client:      r.Client,
			Recorder:    mgr.GetEventRecorder("openmetadataservice-controller"),
			NewOMClient: omclient.NewServiceClient,
		}
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&omv1alpha1.OpenMetadataService{}).
		Named("openmetadataservice").
		Complete(r)
}
