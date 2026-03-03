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

// Package finalizer manages the lifecycle of the OpenMetadata finalizer on
// custom resources. The finalizer prevents Kubernetes from deleting a CR
// before the operator has cleaned up the corresponding entity in OpenMetadata.
package finalizer

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Name is the finalizer string registered on OpenMetadata custom resources.
const Name = "openmetadata.vortexa.com/finalizer"

// IsPresent returns true if the finalizer is already set on the object.
func IsPresent(obj client.Object) bool {
	return controllerutil.ContainsFinalizer(obj, Name)
}

// EnsurePresent adds the finalizer to the object and persists the change.
// Returns (true, nil) if the finalizer was just added, (false, nil) if it was
// already present. Callers should re-reconcile after a true result because the
// underlying object has been updated.
func EnsurePresent(ctx context.Context, c client.Client, obj client.Object) (bool, error) {
	if IsPresent(obj) {
		return false, nil
	}
	controllerutil.AddFinalizer(obj, Name)
	if err := c.Update(ctx, obj); err != nil {
		return false, fmt.Errorf("adding finalizer: %w", err)
	}
	return true, nil
}

// EnsureAbsent removes the finalizer from the object and persists the change.
// If the finalizer is not present, no API call is made.
func EnsureAbsent(ctx context.Context, c client.Client, obj client.Object) error {
	if !IsPresent(obj) {
		return nil
	}
	controllerutil.RemoveFinalizer(obj, Name)
	if err := c.Update(ctx, obj); err != nil {
		return fmt.Errorf("removing finalizer: %w", err)
	}
	return nil
}
