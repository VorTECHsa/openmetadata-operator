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

package omclient

import (
	"errors"
	"fmt"
	"testing"
)

func TestAPIErrorMessage(t *testing.T) {
	err := &APIError{StatusCode: 400, Body: `{"message":"bad request"}`}
	want := `OpenMetadata API returned HTTP 400: {"message":"bad request"}`
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "404 APIError",
			err:  &APIError{StatusCode: 404, Body: "not found"},
			want: true,
		},
		{
			name: "400 APIError",
			err:  &APIError{StatusCode: 400, Body: "bad request"},
			want: false,
		},
		{
			name: "non-APIError",
			err:  errors.New("something else"),
			want: false,
		},
		{
			name: "wrapped 404 APIError",
			err:  fmt.Errorf("context: %w", &APIError{StatusCode: 404, Body: "not found"}),
			want: true, // errors.As unwraps the chain
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNotFound(tt.err)
			if got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}
