package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"modelhub/internal/gateway"
	"modelhub/internal/store"
)

// fakeSource returns a fixed channel list (no database needed).
type fakeSource struct{ chs []*store.Channel }

func (f fakeSource) ChannelsForModel(ctx context.Context, modelID string) ([]*store.Channel, error) {
	return f.chs, nil
}

func newFailoverTestServer(chs []*store.Channel) *Server {
	return &Server{
		provider:      gateway.NewProvider(),
		health:        gateway.NewChannelHealth(),
		channelSource: fakeSource{chs: chs},
	}
}

func upstreamServer(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestDialChatFailover(t *testing.T) {
	srv1 := upstreamServer(500, `{"error":{"message":"boom"}}`)
	defer srv1.Close()
	srv2 := upstreamServer(200, `{"ok":true}`)
	defer srv2.Close()
	s := newFailoverTestServer([]*store.Channel{
		{ID: 1, Name: "c1", BaseURL: srv1.URL, APIKey: "k1"},
		{ID: 2, Name: "c2", BaseURL: srv2.URL, APIKey: "k2"},
	})
	att, err := s.dialChat(context.Background(), "m", []byte(`{}`))
	if err != nil {
		t.Fatalf("dialChat: %v", err)
	}
	defer att.res.Close()
	if att.ch == nil || att.ch.ID != 2 {
		t.Fatalf("expected channel 2, got %+v", att.ch)
	}
	if att.res.StatusCode != 200 {
		t.Fatalf("status = %d", att.res.StatusCode)
	}
	if !s.health.IsDown(1, time.Now()) {
		t.Error("failed channel 1 should be in cooldown")
	}
	if s.health.IsDown(2, time.Now()) {
		t.Error("healthy channel 2 must not be in cooldown")
	}
}

func TestDialChatNonRetryableNoFailover(t *testing.T) {
	hits := 0
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer srv1.Close()
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(200)
	}))
	defer srv2.Close()
	s := newFailoverTestServer([]*store.Channel{
		{ID: 1, Name: "c1", BaseURL: srv1.URL, APIKey: "k1"},
		{ID: 2, Name: "c2", BaseURL: srv2.URL, APIKey: "k2"},
	})
	att, err := s.dialChat(context.Background(), "m", []byte(`{}`))
	if err != nil {
		t.Fatalf("dialChat: %v", err)
	}
	if att.res != nil {
		t.Fatal("400 must be forwarded, not treated as success")
	}
	if att.status != 400 {
		t.Fatalf("status = %d", att.status)
	}
	if hits != 0 {
		t.Fatalf("channel 2 must not be tried for a non-retryable 400 (hits=%d)", hits)
	}
	if !s.health.IsDown(1, time.Now()) {
		// A non-retryable 400 is a request problem, not a channel failure:
		// the channel stays available.
		t.Log("note: non-retryable 400 does not cool the channel down")
	}
}

func TestDialChatTransportErrorFailover(t *testing.T) {
	srv := upstreamServer(200, `{"ok":true}`)
	defer srv.Close()
	s := newFailoverTestServer([]*store.Channel{
		{ID: 1, Name: "c1", BaseURL: "http://127.0.0.1:9", APIKey: "k1"}, // closed port
		{ID: 2, Name: "c2", BaseURL: srv.URL, APIKey: "k2"},
	})
	att, err := s.dialChat(context.Background(), "m", []byte(`{}`))
	if err != nil {
		t.Fatalf("dialChat: %v", err)
	}
	defer att.res.Close()
	if att.ch.ID != 2 {
		t.Fatalf("expected channel 2, got %d", att.ch.ID)
	}
}

func TestDialChatAllCoolingDown(t *testing.T) {
	s := newFailoverTestServer([]*store.Channel{
		{ID: 1, Name: "c1", BaseURL: "http://127.0.0.1:9", APIKey: "k1"},
	})
	s.health.MarkFailed(1, time.Now())
	_, err := s.dialChat(context.Background(), "m", []byte(`{}`))
	if !errors.Is(err, errNoUsableChannel) {
		t.Fatalf("want errNoUsableChannel, got %v", err)
	}
}

func TestDialChatNoChannels(t *testing.T) {
	s := newFailoverTestServer(nil)
	_, err := s.dialChat(context.Background(), "m", []byte(`{}`))
	if !errors.Is(err, errNoUsableChannel) {
		t.Fatalf("want errNoUsableChannel, got %v", err)
	}
}

func TestDialJSONFailover(t *testing.T) {
	srv1 := upstreamServer(401, `{"error":{"message":"invalid key"}}`)
	defer srv1.Close()
	srv2 := upstreamServer(200, "AUDIOBYTES")
	defer srv2.Close()
	s := newFailoverTestServer([]*store.Channel{
		{ID: 1, Name: "c1", BaseURL: srv1.URL, APIKey: "k1"},
		{ID: 2, Name: "c2", BaseURL: srv2.URL, APIKey: "k2"},
	})
	status, body, err := s.dialJSON(context.Background(), "m", http.MethodPost, "/audio/speech", []byte(`{}`), nil)
	if err != nil {
		t.Fatalf("dialJSON: %v", err)
	}
	if status != 200 || string(body) != "AUDIOBYTES" {
		t.Fatalf("status=%d body=%q", status, body)
	}
	if !s.health.IsDown(1, time.Now()) {
		t.Error("channel 1 (401) should be in cooldown")
	}
}

func TestDialJSONExhaustedForwardsLastError(t *testing.T) {
	srv1 := upstreamServer(500, "first boom")
	defer srv1.Close()
	srv2 := upstreamServer(429, "second boom")
	defer srv2.Close()
	s := newFailoverTestServer([]*store.Channel{
		{ID: 1, Name: "c1", BaseURL: srv1.URL, APIKey: "k1"},
		{ID: 2, Name: "c2", BaseURL: srv2.URL, APIKey: "k2"},
	})
	status, body, err := s.dialJSON(context.Background(), "m", http.MethodPost, "/audio/speech", []byte(`{}`), nil)
	if err != nil {
		t.Fatalf("dialJSON: %v", err)
	}
	if status != 429 || string(body) != "second boom" {
		t.Fatalf("want the last retryable error (429), got %d %q", status, body)
	}
}
