// Package client provides a typed Go HTTP client for the confluence
// control API. The base URL is typically read from
// .confluence/current.json by the CLI.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/server"
)

// Client is a typed HTTP client for the confluence /v1 control API.
type Client struct {
	baseURL string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// ErrAPI wraps an api.Error returned by a non-2xx response.
type ErrAPI struct {
	Status int
	Err    api.Error
}

func (e *ErrAPI) Error() string {
	return fmt.Sprintf("api %d: %s (%s)", e.Status, e.Err.Code, e.Err.Message)
}

// New creates a Client targeting baseURL (trailing slash stripped).
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Healthz calls GET /v1/healthz.
func (c *Client) Healthz(ctx context.Context) (server.HealthzResponse, error) {
	var out server.HealthzResponse
	err := c.getJSON(ctx, "/v1/healthz", &out)
	return out, err
}

// Nodes calls GET /v1/nodes.
func (c *Client) Nodes(ctx context.Context) (server.NodesResponse, error) {
	var out server.NodesResponse
	err := c.getJSON(ctx, "/v1/nodes", &out)
	return out, err
}

// StateDiff calls GET /v1/state/diff. If atLedger > 0 the ?at= param is set.
func (c *Client) StateDiff(ctx context.Context, atLedger int) (server.StateDiffResponse, error) {
	path := "/v1/state/diff"
	if atLedger > 0 {
		path += "?at=" + strconv.Itoa(atLedger)
	}
	var out server.StateDiffResponse
	err := c.getJSON(ctx, path, &out)
	return out, err
}

// Findings calls GET /v1/findings with optional filters.
// Empty strings / zero values are omitted from the query.
func (c *Client) Findings(ctx context.Context, since, kind string, limit int) ([]api.Finding, error) {
	q := url.Values{}
	if since != "" {
		q.Set("since", since)
	}
	if kind != "" {
		q.Set("kind", kind)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/v1/findings"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []api.Finding
	err := c.getJSON(ctx, path, &out)
	return out, err
}

// FindingByID calls GET /v1/findings/{id}.
func (c *Client) FindingByID(ctx context.Context, id string) (api.Finding, error) {
	var out api.Finding
	err := c.getJSON(ctx, "/v1/findings/"+url.PathEscape(id), &out)
	return out, err
}

// Scenarios calls GET /v1/scenarios.
func (c *Client) Scenarios(ctx context.Context) (server.ScenarioListResponse, error) {
	var out server.ScenarioListResponse
	err := c.getJSON(ctx, "/v1/scenarios", &out)
	return out, err
}

// ValidateScenario calls POST /v1/scenarios/validate.
func (c *Client) ValidateScenario(ctx context.Context, scenario *api.Scenario) (server.ValidateResponse, error) {
	b, err := json.Marshal(scenario)
	if err != nil {
		return server.ValidateResponse{}, fmt.Errorf("marshal scenario: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/scenarios/validate", bytes.NewReader(b))
	if err != nil {
		return server.ValidateResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return server.ValidateResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return server.ValidateResponse{}, c.apiError(resp)
	}

	var out server.ValidateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return server.ValidateResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

// Logs calls GET /v1/logs and returns the raw NDJSON stream. The caller must
// close the returned ReadCloser. If node is empty the request will be rejected
// by the server; pass at least the node name.
func (c *Client) Logs(ctx context.Context, node string, since time.Duration, grep string, follow bool, limit int) (io.ReadCloser, error) {
	q := url.Values{}
	q.Set("node", node)
	if since > 0 {
		q.Set("since", since.String())
	}
	if grep != "" {
		q.Set("grep", grep)
	}
	if follow {
		q.Set("follow", "true")
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/logs?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/x-ndjson")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, c.apiError(resp)
	}
	return resp.Body, nil
}

// Events calls GET /v1/events and returns the raw SSE stream. The caller must
// close the returned ReadCloser when done.
func (c *Client) Events(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/events", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, c.apiError(resp)
	}
	return resp.Body, nil
}

// getJSON issues a GET, asserts 2xx, and JSON-decodes into v.
func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.apiError(resp)
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// apiError reads a non-2xx response body, attempts to decode an api.ErrorResponse,
// and returns *ErrAPI. If the body is not a recognized envelope it returns a
// generic error with the status and first 200 bytes of body.
func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var envelope api.ErrorResponse
	if json.Unmarshal(body, &envelope) == nil && envelope.Error.Code != "" {
		return &ErrAPI{Status: resp.StatusCode, Err: envelope.Error}
	}

	preview := string(body)
	if len(preview) > 200 {
		preview = preview[:200]
	}
	return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(preview))
}
