package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"modelhub/internal/store"
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