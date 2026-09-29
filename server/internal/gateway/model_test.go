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
func TestPrepareUpstreamBodyInjectsStreamUsage(t *testing.T) {
	in := []byte(`{"model":"public-a","messages":[{"role":"user","content":"hi"}],"temperature":0.7,"stream":true}`)
	out, err := PrepareUpstreamBody(in, "upstream-b", true)
	if err != nil {
		t.Fatalf("PrepareUpstreamBody: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["model"] != "upstream-b" {
		t.Errorf("model = %v, want upstream-b", m["model"])
	}
	opts, ok := m["stream_options"].(map[string]any)
	if !ok {
		t.Fatalf("stream_options missing: %v", m["stream_options"])
	}
	if opts["include_usage"] != true {
		t.Errorf("include_usage = %v, want true", opts["include_usage"])
	}
}

func TestPrepareUpstreamBodyMergesExistingStreamOptions(t *testing.T) {
	in := []byte(`{"model":"a","stream":true,"stream_options":{"keep_inference_detail":true}}`)
	out, err := PrepareUpstreamBody(in, "b", true)
	if err != nil {
		t.Fatalf("PrepareUpstreamBody: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	opts := m["stream_options"].(map[string]any)
	if opts["include_usage"] != true || opts["keep_inference_detail"] != true {
		t.Errorf("stream_options = %v, want both flags", opts)
	}
}

func TestPrepareUpstreamBodyNonStreamLeavesOptions(t *testing.T) {
	in := []byte(`{"model":"a","stream":false}`)
	out, err := PrepareUpstreamBody(in, "b", false)
	if err != nil {
		t.Fatalf("PrepareUpstreamBody: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, exists := m["stream_options"]; exists {
		t.Errorf("stream_options should not be added for non-stream: %v", m)
	}
}
