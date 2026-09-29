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
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
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

const testTools = `[{"type":"function","function":{"name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]`

func TestInjectTools(t *testing.T) {
	body := []byte(`{"model":"agent-weather","stream":true,"messages":[{"role":"user","content":"北京天气?"}]}`)
	out, err := injectTools(body, []byte(testTools))
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tools, ok := parsed["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v, want the injected array", parsed["tools"])
	}
	first, _ := tools[0].(map[string]any)
	fn, _ := first["function"].(map[string]any)
	if fn["name"] != "get_weather" {
		t.Errorf("tool function name = %v, want get_weather", fn["name"])
	}
	if parsed["model"] != "agent-weather" {
		t.Errorf("model not preserved: %v", parsed["model"])
	}

	// a request with its own tools is untouched
	own := []byte(`{"model":"agent-weather","tools":[{"type":"function","function":{"name":"mine","parameters":{}}}]}`)
	out2, err := injectTools(own, []byte(testTools))
	if err != nil {
		t.Fatalf("inject (own tools): %v", err)
	}
	if string(out2) != string(own) {
		t.Errorf("own tools were overridden: %s", out2)
	}

	// explicit null counts as absent
	nullTools := []byte(`{"model":"m","tools":null,"messages":[]}`)
	out3, err := injectTools(nullTools, []byte(testTools))
	if err != nil {
		t.Fatalf("inject (null tools): %v", err)
	}
	var p3 map[string]any
	_ = json.Unmarshal(out3, &p3)
	if _, ok := p3["tools"].([]any); !ok {
		t.Errorf("null tools should be replaced by the template tools")
	}

	if _, err := injectTools([]byte("not json"), []byte(testTools)); err == nil {
		t.Errorf("invalid body: want error, got nil")
	}
}

func TestValidTools(t *testing.T) {
	if err := validTools(nil); err != nil {
		t.Errorf("empty should be valid: %v", err)
	}
	if err := validTools([]byte(testTools)); err != nil {
		t.Errorf("good tools rejected: %v", err)
	}
	if err := validTools([]byte(`"nope"`)); err == nil {
		t.Errorf("non-array should fail")
	}
	if err := validTools([]byte(`[{"type":"function"}]`)); err == nil {
		t.Errorf("missing function object should fail")
	}
	if err := validTools([]byte(`[{"function":"x"}]`)); err == nil {
		t.Errorf("non-object function should fail")
	}
}
