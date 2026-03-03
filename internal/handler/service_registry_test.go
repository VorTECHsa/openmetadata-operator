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
	"testing"

	omv1alpha1 "github.com/VorTECHsa/openmetadata-operator/api/v1alpha1"
)

func TestEndpointForServiceType(t *testing.T) {
	tests := []struct {
		name        string
		serviceType omv1alpha1.ServiceType
		wantEP      string
		wantErr     bool
	}{
		{"database", omv1alpha1.ServiceTypePostgres, endpointDatabaseServices, false},
		{"messaging", omv1alpha1.ServiceTypeKafka, endpointMessagingServices, false},
		{"storage", omv1alpha1.ServiceTypeS3, endpointStorageServices, false},
		{"search", omv1alpha1.ServiceTypeOpenSearch, endpointSearchServices, false},
		{"unsupported type", omv1alpha1.ServiceType("UnsupportedDB"), "", true},
		{"empty string", omv1alpha1.ServiceType(""), "", true},
		{"case sensitive", omv1alpha1.ServiceType("postgres"), "", true}, // lowercase not in registry
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep, err := endpointForServiceType(tt.serviceType)
			if (err != nil) != tt.wantErr {
				t.Errorf("endpointForServiceType(%q) error = %v, wantErr %v", tt.serviceType, err, tt.wantErr)
				return
			}
			if ep != tt.wantEP {
				t.Errorf("endpointForServiceType(%q) = %q, want %q", tt.serviceType, ep, tt.wantEP)
			}
		})
	}
}
