package gateway

import (
	"encoding/json"
	"fmt"
)

// ReplaceModel rewrites the "model" field of an OpenAI chat completions
// request body to the upstream model name, preserving every other field.
func ReplaceModel(body []byte, model string) ([]byte, error) {
	return PrepareUpstreamBody(body, model, false)
}

// PrepareUpstreamBody rewrites the model field and, for stream requests,
// ensures stream_options.include_usage=true so the final SSE chunk carries
// token usage for exact settlement. Existing stream_options are preserved.
func PrepareUpstreamBody(body []byte, model string, stream bool) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	m["model"] = model
	if stream {
		opts, _ := m["stream_options"].(map[string]any)
		if opts == nil {
			opts = map[string]any{}
		}
		opts["include_usage"] = true
		m["stream_options"] = opts
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("re-marshal body: %w", err)
	}
	return out, nil
}

// PeekModel extracts just the model ID from a chat completions request.
func PeekModel(body []byte) (string, error) {
	var m struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return "", fmt.Errorf("invalid JSON body: %w", err)
	}
	if m.Model == "" {
		return "", fmt.Errorf("missing model field")
	}
	return m.Model, nil
}
