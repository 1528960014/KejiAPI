package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kejiapi/internal/store"
)

// ChatResult carries the upstream response so the handler can stream it
// back verbatim and record usage.
type ChatResult struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

// Close releases the upstream body.
func (r *ChatResult) Close() error {
	if r == nil || r.Body == nil {
		return nil
	}
	return r.Body.Close()
}

// Provider talks to upstream model endpoints.
type Provider struct {
	HTTP *http.Client
}

// NewProvider builds a Provider with sane defaults for long-lived SSE streams.
func NewProvider() *Provider {
	return &Provider{
		HTTP: &http.Client{
			Timeout: 0,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

// Chat proxies an OpenAI-compatible chat completions request (the model field
// must already be rewritten to the upstream name) to the channel's endpoint.
func (p *Provider) Chat(ctx context.Context, ch *store.Channel, body []byte) (*ChatResult, error) {
	url := strings.TrimSuffix(ch.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+ch.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")

	resp, err := p.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request: %w", err)
	}
	return &ChatResult{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       resp.Body,
	}, nil
}

// DoJSON performs a JSON request against the channel's base URL and returns
// the upstream status code and raw body. Non-2xx responses are returned (not
// errors) so callers can surface the upstream error payload; only transport
// failures are errors. Extra headers are applied after the defaults.
func (p *Provider) DoJSON(ctx context.Context, ch *store.Channel, method, path string, body []byte, extraHeaders map[string]string) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(ch.BaseURL, "/")+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("build upstream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+ch.APIKey)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("upstream request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return 0, nil, fmt.Errorf("read upstream body: %w", err)
	}
	return resp.StatusCode, raw, nil
}