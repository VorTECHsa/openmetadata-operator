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

// OpenMetadataEntityTagReconciler reconciles an OpenMetadataEntityTag object.
// It is a thin orchestrator: fetch the CR, manage the entity-tag finalizer,
// and delegate the matching/diff/apply loop to the EntityTagHandler.
type OpenMetadataEntityTagReconciler struct {
	client.Client
	Handler *handler.EntityTagHandler
}

// +kubebuilder:rbac:groups=openmetadata.vortexa.com,resources=openmetadataentitytags,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openmetadata.vortexa.com,resources=openmetadataentitytags/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openmetadata.vortexa.com,resources=openmetadataentitytags/finalizers,verbs=update

// Reconcile handles a single reconciliation loop for an OpenMetadataEntityTag resource.
func (r *OpenMetadataEntityTagReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	et := &omv1alpha1.OpenMetadataEntityTag{}
	if err := r.Get(ctx, req.NamespacedName, et); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !et.DeletionTimestamp.IsZero() {
		return r.Handler.HandleDeletion(ctx, et)
	}

	added, err := finalizer.EnsurePresent(ctx, r.Client, et)
	if err != nil {
		return ctrl.Result{}, err
	}
	if added {
		return ctrl.Result{}, nil
	}

	return r.Handler.Reconcile(ctx, et)
}

// SetupWithManager sets up the controller with the Manager. If Handler is nil,
// a default EntityTagHandler is wired with real dependencies.
func (r *OpenMetadataEntityTagReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Handler == nil {
		r.Handler = &handler.EntityTagHandler{
			Client:      r.Client,
			Recorder:    mgr.GetEventRecorder("openmetadataentitytag-controller"),
			NewOMClient: omclient.NewEntityTagClient,
		}
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&omv1alpha1.OpenMetadataEntityTag{}).
		Named("openmetadataentitytag").
		Complete(r)
}
