package gateway

import (
	"encoding/json"
	"testing"
)

func TestReplaceModelPreservesFields(t *testing.T) {
	in := []byte(`{"model":"public-a","messages":[{"role":"user","content":"hi"}],"temperature":0.7,"stream":true}`)
	out, err := ReplaceModel(in, "upstream-b")
	if err != nil {
		t.Fatalf("ReplaceModel: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["model"] != "upstream-b" {
		t.Errorf("model = %v, want upstream-b", m["model"])
	}
	if m["temperature"] != 0.7 {
		t.Errorf("temperature = %v, want 0.7", m["temperature"])
	}
	if m["stream"] != true {
		t.Errorf("stream = %v, want true", m["stream"])
	}
	if len(m["messages"].([]any)) != 1 {
		t.Errorf("messages lost: %v", m["messages"])
	}
}

func TestPeekModel(t *testing.T) {
	model, err := PeekModel([]byte(`{"model":"x","messages":[]}`))
	if err != nil {
		t.Fatalf("PeekModel: %v", err)
	}
	if model != "x" {
		t.Errorf("model = %q, want x", model)
	}
	if _, err := PeekModel([]byte(`{}`)); err == nil {
		t.Error("expected error for missing model")
	}
}