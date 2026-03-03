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

package secretresolver

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resolver resolves secret references within opaque configuration maps.
// It recognises {"valueFrom": {"secretKeyRef": {"name": "...", "key": "..."}}}
// and replaces them with actual values from Kubernetes Secrets.
type Resolver struct {
	client    client.Client
	namespace string
}

// NewResolver creates a resolver scoped to the given namespace.
func NewResolver(c client.Client, namespace string) *Resolver {
	return &Resolver{client: c, namespace: namespace}
}

// Resolve walks the given value recursively. Each config field must use one of:
//
//	{"value": <literal>}       -- unwrapped and returned as-is
//	{"valueFrom": {"secretKeyRef": {"name": "<secret>", "key": "<key>"}}}
//	                           -- resolved from the referenced Kubernetes Secret
//
// This is inspired by the Kubernetes env var secret reference convention
// (value/valueFrom/secretKeyRef) but does not include a "name" field.
// Env vars are open-ended (hashmap) so each entry needs a "name" to
// identify it. Config values are specific (struct) so the field name
// in the config object already serves that purpose.
func (r *Resolver) Resolve(ctx context.Context, value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		if literal, ok := extractValue(v); ok {
			return literal, nil
		}
		if ref, ok := extractSecretKeyRef(v); ok {
			return r.resolveSecretValue(ctx, ref.name, ref.key)
		}

		resolved := make(map[string]any, len(v))
		for k, val := range v {
			resolvedVal, err := r.Resolve(ctx, val)
			if err != nil {
				return nil, fmt.Errorf("resolving key %q: %w", k, err)
			}
			resolved[k] = resolvedVal
		}
		return resolved, nil

	case []any:
		resolved := make([]any, len(v))
		for i, val := range v {
			resolvedVal, err := r.Resolve(ctx, val)
			if err != nil {
				return nil, fmt.Errorf("resolving index %d: %w", i, err)
			}
			resolved[i] = resolvedVal
		}
		return resolved, nil

	default:
		return value, nil
	}
}

// secretKeyRefValue holds the coordinates of a Kubernetes Secret key.
type secretKeyRefValue struct {
	name string
	key  string
}

// extractValue checks whether the map is a literal value wrapper:
// {"value": <anything>}. Returns the unwrapped value and true if matched.
func extractValue(m map[string]any) (any, bool) {
	if len(m) != 1 {
		return nil, false
	}
	v, ok := m["value"]
	return v, ok
}

// extractSecretKeyRef checks whether the map follows the valueFrom pattern:
// {"valueFrom": {"secretKeyRef": {"name": "...", "key": "..."}}}
func extractSecretKeyRef(m map[string]any) (secretKeyRefValue, bool) {
	if len(m) != 1 {
		return secretKeyRefValue{}, false
	}
	valueFrom, ok := m["valueFrom"]
	if !ok {
		return secretKeyRefValue{}, false
	}
	valueFromMap, ok := valueFrom.(map[string]any)
	if !ok {
		return secretKeyRefValue{}, false
	}
	secretKeyRefRaw, ok := valueFromMap["secretKeyRef"]
	if !ok {
		return secretKeyRefValue{}, false
	}
	secretKeyRefMap, ok := secretKeyRefRaw.(map[string]any)
	if !ok {
		return secretKeyRefValue{}, false
	}
	name, nameOk := secretKeyRefMap["name"].(string)
	key, keyOk := secretKeyRefMap["key"].(string)
	if !nameOk || !keyOk {
		return secretKeyRefValue{}, false
	}
	return secretKeyRefValue{name: name, key: key}, true
}

// resolveSecretValue reads a specific key from a Kubernetes Secret.
func (r *Resolver) resolveSecretValue(ctx context.Context, secretName, secretKey string) (string, error) {
	secret := &corev1.Secret{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: r.namespace,
	}, secret)
	if err != nil {
		return "", fmt.Errorf("reading secret %s/%s: %w", r.namespace, secretName, err)
	}

	data, ok := secret.Data[secretKey]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s/%s", secretKey, r.namespace, secretName)
	}

	return string(data), nil
}
