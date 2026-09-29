package api

import "testing"

func TestRealtimeKeyFromProtocols(t *testing.T) {
	// Normal case.
	if got := realtimeKeyFromProtocols([]string{"openai-insecure-api-key.sk-test-123"}); got != "sk-test-123" {
		t.Fatalf("got %q", got)
	}
	// Comma-split header arrives as separate entries; other protocols may precede.
	protos := []string{"something-else", " openai-insecure-api-key.sk-multi ", "another"}
	if got := realtimeKeyFromProtocols(protos); got != "sk-multi" {
		t.Fatalf("got %q", got)
	}
	// No auth subprotocol.
	if got := realtimeKeyFromProtocols([]string{"browser", "chat"}); got != "" {
		t.Fatalf("got %q", got)
	}
	// Empty.
	if got := realtimeKeyFromProtocols(nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestRealtimeSessionModel(t *testing.T) {
	model, ok := realtimeSessionModel([]byte(
		`{"type":"session.update","session":{"model":"gpt-realtime-1","voice":"alloy"}}`))
	if !ok || model != "gpt-realtime-1" {
		t.Fatalf("got %q, ok=%v", model, ok)
	}
	// Non-session.update frames do not name a model.
	if _, ok := realtimeSessionModel([]byte(`{"type":"input_audio_buffer.append","audio":"AA=="}`)); ok {
		t.Fatal("audio append should not match")
	}
	// session.update without a model keeps buffering.
	if _, ok := realtimeSessionModel([]byte(`{"type":"session.update","session":{"voice":"alloy"}}`)); ok {
		t.Fatal("missing model should not match")
	}
	// Malformed JSON.
	if _, ok := realtimeSessionModel([]byte("not json")); ok {
		t.Fatal("malformed frame should not match")
	}
}

func TestRealtimeUsageFromFrame(t *testing.T) {
	// Cost reported directly (preferred).
	cost, in, out, ok := realtimeUsageFromFrame([]byte(`{"type":"response.done","response":{"usage":{"cost":0.1234,"input_tokens":100,"output_tokens":50}}}`))
	if !ok || cost != 0.1234 || in != 100 || out != 50 {
		t.Fatalf("got cost=%v in=%d out=%d ok=%v", cost, in, out, ok)
	}
	// No cost: token fallback.
	cost, in, out, ok = realtimeUsageFromFrame([]byte(`{"type":"response.done","response":{"usage":{"input_tokens":120,"output_tokens":80}}}`))
	if !ok || cost != 0 || in != 120 || out != 80 {
		t.Fatalf("got cost=%v in=%d out=%d ok=%v", cost, in, out, ok)
	}
	// Older SDK field names.
	cost, in, out, ok = realtimeUsageFromFrame([]byte(`{"type":"response.done","response":{"usage":{"input_token_count":7,"output_token_count":3}}}`))
	if !ok || cost != 0 || in != 7 || out != 3 {
		t.Fatalf("got cost=%v in=%d out=%d ok=%v", cost, in, out, ok)
	}
	// Other event types.
	if _, _, _, ok := realtimeUsageFromFrame([]byte(`{"type":"response.audio.delta","delta":"AA=="}`)); ok {
		t.Fatal("audio delta should not match")
	}
	// No usage at all.
	if _, _, _, ok := realtimeUsageFromFrame([]byte(`{"type":"response.done","response":{}}`)); ok {
		t.Fatal("empty usage should not match")
	}
}

func TestRealtimeUpstreamURL(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com":       "wss://api.openai.com/realtime",
		"https://api.openai.com/":      "wss://api.openai.com/realtime",
		"http://127.0.0.1:8080":        "ws://127.0.0.1:8080/realtime",
		"https://relay.example.com/v1": "wss://relay.example.com/v1/realtime",
	}
	for base, want := range cases {
		if got := realtimeUpstreamURL(base); got != want {
			t.Errorf("%s: got %q want %q", base, got, want)
		}
	}
	// Garbage input yields no URL.
	if got := realtimeUpstreamURL("://nope"); got != "" {
		t.Fatalf("got %q", got)
	}
}
