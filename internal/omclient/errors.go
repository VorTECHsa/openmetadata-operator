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
	"net/http"
)

// APIError represents an unexpected HTTP status from the OpenMetadata API.
type APIError struct {
	// StatusCode is the HTTP status code returned by the API.
	StatusCode int
	// Body is the raw response body, useful for debugging.
	Body string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("OpenMetadata API returned HTTP %d: %s", e.StatusCode, e.Body)
}

// Compile-time check that *APIError implements the error interface.
var _ error = (*APIError)(nil)

// IsNotFound reports whether the error (or any wrapped error in its chain)
// represents an HTTP 404 response.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}
