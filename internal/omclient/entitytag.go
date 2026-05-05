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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// searchPageSize is how many hits to request per search page.
const searchPageSize = 1000

// fqnField is the OpenMetadata search field used for FQN matching. It's
// indexed as an exact-match keyword with a lowercase normaliser — wildcards
// match the full lowercased string, so there's no tokenisation surprise.
const fqnField = "fullyQualifiedName"

// NewEntityTagClient creates a new OpenMetadata API client for entity-tag operations.
func NewEntityTagClient(baseURL, token string) EntityTagClient {
	return newClient(baseURL, token)
}

// Compile-time check.
var _ EntityTagClient = (*Client)(nil)

// SearchEntities returns every entity in the named search index whose
// fullyQualifiedName matches any include pattern and no exclude pattern.
// Wildcards '*' (zero or more chars) and '?' (one char) in patterns are
// passed through to OpenMetadata's search endpoint unchanged. Pagination is
// handled internally.
func (c *Client) SearchEntities(ctx context.Context, searchIndex string, includes, excludes []string) ([]EntitySummary, error) {
	if len(includes) == 0 {
		return nil, nil
	}
	q := buildSearchQuery(includes, excludes)

	var (
		out  []EntitySummary
		from int
	)
	for {
		params := url.Values{}
		params.Set("q", q)
		params.Set("index", searchIndex)
		params.Set("size", fmt.Sprintf("%d", searchPageSize))
		params.Set("from", fmt.Sprintf("%d", from))
		reqURL := fmt.Sprintf("%s/v1/search/query?%s", c.baseURL, params.Encode())

		page, err := c.fetchSearchPage(ctx, reqURL)
		if err != nil {
			return nil, err
		}
		for _, h := range page.Hits.Hits {
			out = append(out, h.Source)
		}
		if len(page.Hits.Hits) < searchPageSize {
			break
		}
		from += searchPageSize
		// Defensive cap to avoid runaway pagination on a misbehaving backend.
		if from >= page.Hits.Total.Value {
			break
		}
	}
	return out, nil
}

func (c *Client) fetchSearchPage(ctx context.Context, reqURL string) (*searchResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("search entities: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var page searchResponse
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("decoding search response: %w", err)
	}
	return &page, nil
}

// buildSearchQuery composes the search query string sent to OpenMetadata in
// the form
//
//	(field:p1 OR field:p2) AND NOT (field:e1 OR field:e2)
//
// where field = "fullyQualifiedName". Special characters in the literal
// portion of each pattern are escaped; '*' and '?' are passed through so
// they retain their wildcard meaning.
func buildSearchQuery(includes, excludes []string) string {
	includeClause := joinPatternClauses(includes)
	if len(excludes) == 0 {
		return includeClause
	}
	excludeClause := joinPatternClauses(excludes)
	return fmt.Sprintf("%s AND NOT %s", includeClause, excludeClause)
}

func joinPatternClauses(patterns []string) string {
	parts := make([]string, 0, len(patterns))
	for _, p := range patterns {
		parts = append(parts, fmt.Sprintf("%s:%s", fqnField, escapeQueryValue(p)))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// escapeQueryValue escapes characters reserved by OpenMetadata's search
// query syntax inside a pattern, while preserving the wildcards '*' and '?'.
// Reserved chars:
//
//   - - = & | > < ! ( ) { } [ ] ^ " ~ : \ /
//
// The whole-word "AND", "OR", "NOT" operators are also reserved but cannot
// appear inside our keyword-field values, so we don't worry about them.
func escapeQueryValue(s string) string {
	const reserved = `+-=&|><!(){}[]^"~:\/`
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if strings.ContainsRune(reserved, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// BulkAddTagToAssets calls PUT /api/v1/tags/{tagId}/assets/add.
// dryRun is forced to false (OM's API defaults it to true).
func (c *Client) BulkAddTagToAssets(ctx context.Context, tagID string, assets []AssetRef) error {
	return c.bulkTagAssetOp(ctx, tagID, "add", assets)
}

// BulkRemoveTagFromAssets calls PUT /api/v1/tags/{tagId}/assets/remove.
func (c *Client) BulkRemoveTagFromAssets(ctx context.Context, tagID string, assets []AssetRef) error {
	return c.bulkTagAssetOp(ctx, tagID, "remove", assets)
}

func (c *Client) bulkTagAssetOp(ctx context.Context, tagID, op string, assets []AssetRef) error {
	if len(assets) == 0 {
		return nil
	}
	reqURL := fmt.Sprintf("%s/v1/tags/%s/assets/%s", c.baseURL, url.PathEscape(tagID), op)

	resp, err := c.doRequest(ctx, http.MethodPut, reqURL, AddTagToAssetsRequest{
		DryRun: false,
		Assets: assets,
	})
	if err != nil {
		return fmt.Errorf("bulk tag-asset %s: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading bulk tag-asset %s response: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	return nil
}
