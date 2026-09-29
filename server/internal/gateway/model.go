package gateway

import (
	"encoding/json"
	"fmt"
)

// ReplaceModel rewrites the "model" field of an OpenAI chat completions
// request body to the upstream model name, preserving every other field.
func ReplaceModel(body []byte, model string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	m["model"] = model
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