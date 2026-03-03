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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
)

// resolveAuthToken reads the JWT token from the referenced Secret.
func resolveAuthToken(ctx context.Context, k8sClient client.Client, ref omv1alpha1.SecretReference) (string, error) {
	secret := &corev1.Secret{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}, secret)
	if err != nil {
		return "", fmt.Errorf("reading auth secret %s/%s: %w", ref.Namespace, ref.Name, err)
	}

	token, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %q not found in auth secret %s/%s", ref.Key, ref.Namespace, ref.Name)
	}

	return string(token), nil
}
