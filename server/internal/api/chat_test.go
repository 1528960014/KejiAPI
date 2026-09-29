package api

import (
	"encoding/json"
	"testing"
)

func TestInjectSystemPrompt(t *testing.T) {
	body := []byte(`{"model":"agent-translator","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	out, err := injectSystemPrompt(body, "You are a translator.")
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	var parsed struct {
		Model   string `json:"model"`
		Stream  bool   `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Model != "agent-translator" || !parsed.Stream {
		t.Errorf("fields not preserved: %+v", parsed)
	}
	if len(parsed.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(parsed.Messages))
	}
	if parsed.Messages[0].Role != "system" || parsed.Messages[0].Content != "You are a translator." {
		t.Errorf("first message = %+v, want the injected system prompt", parsed.Messages[0])
	}
	if parsed.Messages[1].Role != "user" || parsed.Messages[1].Content != "hi" {
		t.Errorf("second message = %+v, want the original user message", parsed.Messages[1])
	}

	// invalid JSON must fail, not silently pass through
	if _, err := injectSystemPrompt([]byte("not json"), "x"); err == nil {
		t.Errorf("invalid body: want error, got nil")
	}
}
