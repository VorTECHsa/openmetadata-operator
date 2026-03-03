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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const httpTimeout = 30 * time.Second

// Compile-time checks: *Client must satisfy all client interfaces.
var (
	_ ServiceClient  = (*Client)(nil)
	_ PipelineClient = (*Client)(nil)
	_ TestCaseClient = (*Client)(nil)
)

// Client is a thin REST client for the OpenMetadata API.
// It is created fresh for each reconciliation — no long-lived state.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewServiceClient creates a new OpenMetadata API client for service operations.
func NewServiceClient(baseURL, token string) ServiceClient {
	return newClient(baseURL, token)
}

// NewPipelineClient creates a new OpenMetadata API client for pipeline operations.
func NewPipelineClient(baseURL, token string) PipelineClient {
	return newClient(baseURL, token)
}

// NewTestCaseClient creates a new OpenMetadata API client for test case operations.
func NewTestCaseClient(baseURL, token string) TestCaseClient {
	return newClient(baseURL, token)
}

func newClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
	}
}

// UpsertService creates or updates a service via PUT.
// OpenMetadata PUT endpoints are idempotent upserts.
func (c *Client) UpsertService(ctx context.Context, endpoint string, req ServiceRequest) (*ServiceResponse, error) {
	reqURL := fmt.Sprintf("%s/v1/services/%s", c.baseURL, endpoint)

	resp, err := c.doRequest(ctx, http.MethodPut, reqURL, req)
	if err != nil {
		return nil, fmt.Errorf("upsert service: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading upsert response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var svcResp ServiceResponse
	if err := json.Unmarshal(body, &svcResp); err != nil {
		return nil, fmt.Errorf("decoding upsert response: %w", err)
	}
	return &svcResp, nil
}

// GetServiceByName retrieves a service by its fully qualified name.
// Returns an *APIError with status 404 if the service does not exist.
func (c *Client) GetServiceByName(ctx context.Context, endpoint, fqn string) (*ServiceResponse, error) {
	reqURL := fmt.Sprintf("%s/v1/services/%s/name/%s", c.baseURL, endpoint, url.PathEscape(fqn))

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("get service by name: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading get response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var svcResp ServiceResponse
	if err := json.Unmarshal(body, &svcResp); err != nil {
		return nil, fmt.Errorf("decoding get response: %w", err)
	}
	return &svcResp, nil
}

// DeleteService deletes a service by ID with recursive and hard delete.
// This cascades to all child entities (databases, schemas, tables, pipelines).
func (c *Client) DeleteService(ctx context.Context, endpoint, id string) error {
	reqURL := fmt.Sprintf("%s/v1/services/%s/%s?recursive=true&hardDelete=true", c.baseURL, endpoint, id)

	resp, err := c.doRequest(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading delete response: %w", err)
	}

	// Treat 404 as success — the service is already gone.
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		return nil
	}

	return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
}

// UpsertPipeline creates or updates an ingestion pipeline via PUT.
func (c *Client) UpsertPipeline(ctx context.Context, req PipelineRequest) (*PipelineResponse, error) {
	reqURL := fmt.Sprintf("%s/v1/services/ingestionPipelines", c.baseURL)

	resp, err := c.doRequest(ctx, http.MethodPut, reqURL, req)
	if err != nil {
		return nil, fmt.Errorf("upsert pipeline: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading upsert pipeline response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var pResp PipelineResponse
	if err := json.Unmarshal(body, &pResp); err != nil {
		return nil, fmt.Errorf("decoding upsert pipeline response: %w", err)
	}
	return &pResp, nil
}

// GetPipelineByFQN retrieves a pipeline by fully qualified name.
// Returns an *APIError with status 404 if the pipeline does not exist.
func (c *Client) GetPipelineByFQN(ctx context.Context, fqn string) (*PipelineResponse, error) {
	reqURL := fmt.Sprintf("%s/v1/services/ingestionPipelines/name/%s", c.baseURL, url.PathEscape(fqn))

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("get pipeline by FQN: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading get pipeline response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var pResp PipelineResponse
	if err := json.Unmarshal(body, &pResp); err != nil {
		return nil, fmt.Errorf("decoding get pipeline response: %w", err)
	}
	return &pResp, nil
}

// DeployPipeline triggers Airflow DAG deployment for the given pipeline ID.
func (c *Client) DeployPipeline(ctx context.Context, id string) error {
	reqURL := fmt.Sprintf("%s/v1/services/ingestionPipelines/deploy/%s", c.baseURL, id)

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return fmt.Errorf("deploy pipeline: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading deploy pipeline response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	return nil
}

// DeletePipeline deletes a pipeline by ID with hard delete.
func (c *Client) DeletePipeline(ctx context.Context, id string) error {
	reqURL := fmt.Sprintf("%s/v1/services/ingestionPipelines/%s?hardDelete=true", c.baseURL, id)

	resp, err := c.doRequest(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("delete pipeline: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading delete pipeline response: %w", err)
	}

	// Treat 404 as success — the pipeline is already gone.
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		return nil
	}

	return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
}

// GetEntityByName retrieves any entity by type path and FQN, returning its ID.
// Used to resolve FQN-based references to OM UUIDs.
// Example paths: "services/databaseServices", "dataQuality/testSuites".
func (c *Client) GetEntityByName(ctx context.Context, entityTypePath, fqn string) (string, error) {
	reqURL := fmt.Sprintf("%s/v1/%s/name/%s", c.baseURL, entityTypePath, url.PathEscape(fqn))

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("get entity by name: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading get entity response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var entity entityIDResponse
	if err := json.Unmarshal(body, &entity); err != nil {
		return "", fmt.Errorf("decoding entity response: %w", err)
	}
	return entity.ID, nil
}

// UpsertTestCase creates or updates a test case via PUT.
func (c *Client) UpsertTestCase(ctx context.Context, req TestCaseRequest) (*TestCaseResponse, error) {
	reqURL := fmt.Sprintf("%s/v1/dataQuality/testCases", c.baseURL)

	resp, err := c.doRequest(ctx, http.MethodPut, reqURL, req)
	if err != nil {
		return nil, fmt.Errorf("upsert test case: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading upsert test case response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var tcResp TestCaseResponse
	if err := json.Unmarshal(body, &tcResp); err != nil {
		return nil, fmt.Errorf("decoding upsert test case response: %w", err)
	}
	return &tcResp, nil
}

// GetTestCaseByFQN retrieves a test case by fully qualified name.
// Returns an *APIError with status 404 if the test case does not exist.
func (c *Client) GetTestCaseByFQN(ctx context.Context, fqn string) (*TestCaseResponse, error) {
	reqURL := fmt.Sprintf("%s/v1/dataQuality/testCases/name/%s", c.baseURL, url.PathEscape(fqn))

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("get test case by FQN: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading get test case response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var tcResp TestCaseResponse
	if err := json.Unmarshal(body, &tcResp); err != nil {
		return nil, fmt.Errorf("decoding get test case response: %w", err)
	}
	return &tcResp, nil
}

// DeleteTestCase deletes a test case by ID with hard delete.
func (c *Client) DeleteTestCase(ctx context.Context, id string) error {
	reqURL := fmt.Sprintf("%s/v1/dataQuality/testCases/%s?hardDelete=true", c.baseURL, id)

	resp, err := c.doRequest(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("delete test case: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading delete test case response: %w", err)
	}

	// Treat 404 as success — the test case is already gone.
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		return nil
	}

	return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
}

// doRequest builds and executes an HTTP request with auth headers.
func (c *Client) doRequest(ctx context.Context, method, reqURL string, payload any) (*http.Response, error) {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshalling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}
