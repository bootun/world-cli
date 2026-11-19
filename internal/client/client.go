package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Int64 encodes 64-bit integers as decimal strings in JSON and accepts either
// JSON strings or numbers when decoding.
type Int64 int64

func (v Int64) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatInt(int64(v), 10))
}

func (v *Int64) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*v = 0
		return nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			*v = 0
			return nil
		}
		parsed, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		*v = Int64(parsed)
		return nil
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return err
	}
	*v = Int64(parsed)
	return nil
}

type APIError struct {
	HTTPStatus int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Code, e.Message, e.HTTPStatus)
	}
	if e.Message != "" {
		return fmt.Sprintf("%s (HTTP %d)", e.Message, e.HTTPStatus)
	}
	return fmt.Sprintf("HTTP %d", e.HTTPStatus)
}

type WriteMemoryRequest struct {
	Text           string         `json:"text"`
	OccurredAtMs   *Int64         `json:"occurredAtMs,omitempty"`
	Source         string         `json:"source,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Timezone       string         `json:"timezone,omitempty"`
	IdempotencyKey string         `json:"-"`
}

type WriteMemoryResponse struct {
	EventID      string `json:"eventId"`
	Deduplicated bool   `json:"deduplicated"`
}

type ComponentView struct {
	ComponentType   string         `json:"componentType"`
	SchemaVersion   string         `json:"schemaVersion"`
	Payload         map[string]any `json:"payload"`
	ValidFromMs     Int64          `json:"validFromMs"`
	ValidToMs       Int64          `json:"validToMs"`
	Confidence      *float64       `json:"confidence,omitempty"`
	EpistemicStatus string         `json:"epistemicStatus"`
}

type EntityView struct {
	EntityID      string                   `json:"entityId"`
	CanonicalName string                   `json:"canonicalName"`
	EntityType    string                   `json:"entityType"`
	Aliases       []string                 `json:"aliases,omitempty"`
	Components    map[string]ComponentView `json:"components,omitempty"`
}

type GetEntityResponse struct {
	Entity *EntityView `json:"entity"`
}

type RecallRequest struct {
	Query           string
	MaxEntities     uint32
	MaxEvents       uint32
	AsOfValidTimeMs int64
}

type MemoryView struct {
	EventID           string   `json:"eventId"`
	OccurredAtMs      Int64    `json:"occurredAtMs"`
	Summary           string   `json:"summary"`
	AffectedEntityIds []string `json:"affectedEntityIds,omitempty"`
}

type RecallResponse struct {
	Entities []EntityView `json:"entities"`
	Memories []MemoryView `json:"memories"`
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	userAgent  string
}

type Option func(*Client)

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

func New(baseURL, apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		userAgent: "world-cli/0.1",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) WriteMemory(ctx context.Context, req WriteMemoryRequest) (WriteMemoryResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return WriteMemoryResponse{}, err
	}
	headers := map[string]string{}
	if req.IdempotencyKey != "" {
		headers["Idempotency-Key"] = req.IdempotencyKey
	}

	var out WriteMemoryResponse
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = c.doJSON(ctx, http.MethodPost, "/v1/memories", body, headers, &out)
		if err == nil {
			return out, nil
		}
		if attempt == maxAttempts-1 || !isRetryable(err) {
			return WriteMemoryResponse{}, err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return WriteMemoryResponse{}, ctx.Err()
		case <-timer.C:
		}
	}
	return WriteMemoryResponse{}, err
}

func (c *Client) Recall(ctx context.Context, req RecallRequest) (RecallResponse, error) {
	q := url.Values{}
	q.Set("query", req.Query)
	if req.MaxEntities > 0 {
		q.Set("maxEntities", strconv.FormatUint(uint64(req.MaxEntities), 10))
	}
	if req.MaxEvents > 0 {
		q.Set("maxEvents", strconv.FormatUint(uint64(req.MaxEvents), 10))
	}
	if req.AsOfValidTimeMs != 0 {
		q.Set("asOfValidTimeMs", strconv.FormatInt(req.AsOfValidTimeMs, 10))
	}
	var out RecallResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/memories:recall?"+q.Encode(), nil, nil, &out)
	if out.Entities == nil {
		out.Entities = []EntityView{}
	}
	if out.Memories == nil {
		out.Memories = []MemoryView{}
	}
	return out, err
}

func (c *Client) GetEntity(ctx context.Context, entityID string, asOfValidTimeMs int64) (GetEntityResponse, error) {
	path := "/v1/entities/" + url.PathEscape(entityID)
	if asOfValidTimeMs != 0 {
		q := url.Values{}
		q.Set("asOfValidTimeMs", strconv.FormatInt(asOfValidTimeMs, 10))
		path += "?" + q.Encode()
	}
	var out GetEntityResponse
	err := c.doJSON(ctx, http.MethodGet, path, nil, nil, &out)
	return out, err
}

func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode >= 400 {
		return &APIError{HTTPStatus: res.StatusCode, Message: "health check failed"}
	}
	return nil
}

// VerifyAuth checks whether the configured API key is accepted by the server.
func (c *Client) VerifyAuth(ctx context.Context) error {
	var out GetEntityResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/entities/__world_cli_auth_probe__", nil, nil, &out)
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.HTTPStatus == http.StatusNotFound || apiErr.Code == "not_found") {
		return nil
	}
	return err
}

func (c *Client) doJSON(ctx context.Context, method, path string, body []byte, extraHeaders map[string]string, target any) error {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return parseAPIError(res.StatusCode, raw)
	}
	if target == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}

func parseAPIError(status int, raw []byte) error {
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &env); err == nil {
		if env.Error.Code != "" || env.Error.Message != "" {
			return &APIError{HTTPStatus: status, Code: env.Error.Code, Message: env.Error.Message}
		}
		if env.Message != "" {
			return &APIError{HTTPStatus: status, Message: env.Message}
		}
	}
	msg := strings.TrimSpace(string(raw))
	if len(msg) > 256 {
		msg = msg[:256] + "..."
	}
	return &APIError{HTTPStatus: status, Message: msg}
}

func isRetryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatus == http.StatusServiceUnavailable || apiErr.HTTPStatus == http.StatusTooManyRequests || apiErr.Code == "unavailable"
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
