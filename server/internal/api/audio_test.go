package api

import (
	"encoding/json"
	"testing"

	"kejiapi/internal/store"
)

func TestSpeechContentType(t *testing.T) {
	cases := map[string]string{
		"mp3":  "audio/mpeg",
		"opus": "audio/ogg",
		"aac":  "audio/aac",
		"flac": "audio/flac",
		"wav":  "audio/wav",
		"pcm":  "audio/L16",
	}
	for format, want := range cases {
		got, ok := speechContentType(format)
		if !ok || got != want {
			t.Errorf("speechContentType(%q) = %q, %v; want %q", format, got, ok, want)
		}
	}
	if _, ok := speechContentType("ogg"); ok {
		t.Errorf("unknown format must not resolve")
	}
}

func TestSpeechUpstreamBody(t *testing.T) {
	m := &store.Model{UpstreamModel: "tts-1-hd"}
	req := &speechRequest{Model: "tts-1", Input: "hello world"}
	var parsed struct {
		Model          string   `json:"model"`
		Input          string   `json:"input"`
		Voice          string   `json:"voice"`
		ResponseFormat string   `json:"response_format"`
		Speed          *float64 `json:"speed"`
	}
	if err := json.Unmarshal(speechUpstreamBody(m, req), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Model != "tts-1-hd" {
		t.Errorf("model = %q, want the upstream name", parsed.Model)
	}
	if parsed.Input != "hello world" {
		t.Errorf("input = %q", parsed.Input)
	}
	if parsed.ResponseFormat != "mp3" {
		t.Errorf("response_format = %q, want mp3 default", parsed.ResponseFormat)
	}
	if parsed.Speed != nil {
		t.Errorf("speed should be omitted when unset")
	}

	speed := 1.5
	req2 := &speechRequest{Model: "tts-1", Input: "x", Voice: "alloy", ResponseFormat: "wav", Speed: &speed}
	var p2 map[string]any
	_ = json.Unmarshal(speechUpstreamBody(m, req2), &p2)
	if p2["voice"] != "alloy" || p2["response_format"] != "wav" || p2["speed"] != 1.5 {
		t.Errorf("voice/format/speed not forwarded: %v", p2)
	}
}