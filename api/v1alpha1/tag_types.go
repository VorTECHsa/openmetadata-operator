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

package v1alpha1

// TagRef references a tag in OpenMetadata by its fully qualified name
// (e.g. "Tier.Tier3"). The operator validates the tag exists at reconcile time.
type TagRef struct {
	// TagFQN is the fully qualified name of the tag, in the form
	// "<classification>.<tagName>" (e.g. "Tier.Tier3", "PII.Sensitive").
	// +kubebuilder:validation:MinLength=1
	TagFQN string `json:"tagFQN"`
}
