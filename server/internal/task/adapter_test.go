package task

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"kejiapi/internal/gateway"
	"kejiapi/internal/store"
)

func newTestProvider(base string) *gateway.Provider {
	return &gateway.Provider{HTTP: &http.Client{Timeout: 5 * time.Second}}
}

func testChannel(base string) *store.Channel {
	return &store.Channel{ID: 1, Name: "test", Provider: "test", BaseURL: base, APIKey: "sk-test", ModelID: "m", Priority: 1, Enabled: true}
}

func testModel() *store.Model {
	return &store.Model{ModelID: "m", Provider: "test", UpstreamModel: "up-m", Capabilities: []string{"image"}, PriceUnit: "image", UnitPrice: 0.02}
}

func TestOpenAIImageAdapterSuccess(t *testing.T) {
	var gotModel, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Errorf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel, _ = req["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[{"url":"https://cdn/x.png"},{"url":"https://cdn/y.png"}]}`))
	}))
	defer srv.Close()

	p := newTestProvider(srv.URL)
	urls, err := (openaiImageAdapter{}).Run(context.Background(), p, testChannel(srv.URL), testModel(), []byte(`{"prompt":"a cat","n":2,"size":"512x512"}`))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(urls) != 2 || urls[0] != "https://cdn/x.png" {
		t.Errorf("urls = %v", urls)
	}
	if gotModel != "up-m" {
		t.Errorf("model not rewritten: %q", gotModel)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth header = %q", gotAuth)
	}
}

func TestSunoMusicAdapterSuccess(t *testing.T) {
	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/generate":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req["text"] != "a chill track" {
				t.Errorf("text = %v", req["text"])
			}
			_, _ = w.Write([]byte(`{"id":"suno-1"}`))
		case "/api/v1/generate/suno-1":
			if n := atomic.AddInt32(&polls, 1); n == 1 {
				_, _ = w.Write([]byte(`{"id":"suno-1","status":"processing"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"suno-1","status":"complete","audio_url":"https://cdn/suno-1.mp3"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	old := pollInterval
	pollInterval = time.Millisecond
	defer func() { pollInterval = old }()

	m := &store.Model{ModelID: "m", Provider: "suno", UpstreamModel: "v3.5", Capabilities: []string{"music"}, PriceUnit: "music", UnitPrice: 0.05}
	urls, err := (sunoMusicAdapter{}).Run(context.Background(), newTestProvider(srv.URL), testChannel(srv.URL), m, []byte(`{"prompt":"a chill track"}`))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(urls) != 1 || urls[0] != "https://cdn/suno-1.mp3" {
		t.Errorf("urls = %v", urls)
	}
	if atomic.LoadInt32(&polls) < 2 {
		t.Errorf("polls = %d, want >= 2 (queued then complete)", polls)
	}
}

func TestSunoMusicAdapterFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/generate":
			_, _ = w.Write([]byte(`{"id":"suno-2"}`))
		default:
			_, _ = w.Write([]byte(`{"id":"suno-2","status":"failed","fail_reason":"safety"}`))
		}
	}))
	defer srv.Close()

	_, err := (sunoMusicAdapter{}).Run(context.Background(), newTestProvider(srv.URL), testChannel(srv.URL), testModel(), []byte(`{"prompt":"x"}`))
	if err == nil || !containsString(err.Error(), "failed") {
		t.Fatalf("want upstream failed error, got %v", err)
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestOpenAIImageAdapterB64Fails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aGVsbG8="}]}`))
	}))
	defer srv.Close()

	_, err := (openaiImageAdapter{}).Run(context.Background(), newTestProvider(srv.URL), testChannel(srv.URL), testModel(), []byte(`{"prompt":"a cat"}`))
	if err == nil {
		t.Fatal("want error for b64_json without object storage")
	}
}

func TestOpenAIImageAdapterUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream down"}`))
	}))
	defer srv.Close()

	_, err := (openaiImageAdapter{}).Run(context.Background(), newTestProvider(srv.URL), testChannel(srv.URL), testModel(), []byte(`{"prompt":"a cat"}`))
	if err == nil {
		t.Fatal("want error for HTTP 502")
	}
}

func TestDashscopeVideoAdapterAsyncFlow(t *testing.T) {
	pollInterval = time.Millisecond
	t.Cleanup(func() { pollInterval = 5 * time.Second })

	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services/aigc/video-generation/video-synthesis":
			if r.Header.Get("X-DashScope-Async") != "enable" {
				t.Error("missing X-DashScope-Async header")
			}
			_, _ = w.Write([]byte(`{"output":{"task_id":"tsk-1","task_status":"PENDING"},"request_id":"r1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/tsk-1":
			n := atomic.AddInt32(&polls, 1)
			if n < 2 {
				_, _ = w.Write([]byte(`{"output":{"task_id":"tsk-1","task_status":"RUNNING"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"output":{"task_id":"tsk-1","task_status":"SUCCEEDED","video_url":"https://cdn/v.mp4"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ch := testChannel(srv.URL)
	ch.Provider = "dashscope"
	m := testModel()
	m.UpstreamModel = "wanx-video"

	urls, err := (dashscopeVideoAdapter{}).Run(context.Background(), newTestProvider(srv.URL), ch, m, []byte(`{"prompt":"ocean waves","duration":"5"}`))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(urls) != 1 || urls[0] != "https://cdn/v.mp4" {
		t.Errorf("urls = %v", urls)
	}
	if atomic.LoadInt32(&polls) != 2 {
		t.Errorf("polls = %d, want 2", polls)
	}
}

func TestDashscopeVideoAdapterUpstreamFailure(t *testing.T) {
	pollInterval = time.Millisecond
	t.Cleanup(func() { pollInterval = 5 * time.Second })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"output":{"task_id":"tsk-2","task_status":"FAILED","message":"content policy"}`))
	}))
	defer srv.Close()

	ch := testChannel(srv.URL)
	ch.Provider = "dashscope"
	_, err := (dashscopeVideoAdapter{}).Run(context.Background(), newTestProvider(srv.URL), ch, testModel(), []byte(`{"prompt":"x"}`))
	if err == nil {
		t.Fatal("want error for FAILED task")
	}
}

func TestKlingVideoAdapterFlow(t *testing.T) {
	pollInterval = time.Millisecond
	t.Cleanup(func() { pollInterval = 5 * time.Second })

	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/videos/text2video":
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"task_info":{"id":"klg-1","task_status":"submitted"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/text2video/klg-1":
			n := atomic.AddInt32(&polls, 1)
			if n == 1 {
				_, _ = w.Write([]byte(`{"data":{"task_info":{"task_status":"processing"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"task_info":{"task_status":"succeeded","task_result":{"videos":[{"id":"vid","url":"https://cdn/k.mp4"}]}}}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ch := testChannel(srv.URL)
	ch.Provider = "kling"
	urls, err := (klingVideoAdapter{}).Run(context.Background(), newTestProvider(srv.URL), ch, testModel(), []byte(`{"prompt":"a dog"}`))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(urls) != 1 || urls[0] != "https://cdn/k.mp4" {
		t.Errorf("urls = %v", urls)
	}
}

func TestKlingVideoAdapterFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"task_info":{"task_status":"failed","fail_reason":"prompt rejected"}}}`))
	}))
	defer srv.Close()

	ch := testChannel(srv.URL)
	ch.Provider = "kling"
	_, err := (klingVideoAdapter{}).Run(context.Background(), newTestProvider(srv.URL), ch, testModel(), []byte(`{"prompt":"x"}`))
	if err == nil {
		t.Fatal("want error for failed kling task")
	}
}

func TestDashscopeTTSAdapter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/services/aigc/multimodal-generation/generation" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"output":{"audio":{"url":"https://cdn/a.wav"}}}`))
	}))
	defer srv.Close()

	ch := testChannel(srv.URL)
	ch.Provider = "dashscope"
	urls, err := (dashscopeTTSAdapter{}).Run(context.Background(), newTestProvider(srv.URL), ch, testModel(), []byte(`{"text":"hello world"}`))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(urls) != 1 || urls[0] != "https://cdn/a.wav" {
		t.Errorf("urls = %v", urls)
	}
}

func TestAdapterRegistry(t *testing.T) {
	cases := []struct {
		provider, taskType string
		wantNil            bool
	}{
		{"openai", "image", false},
		{"siliconflow", "image", false},
		{"dashscope", "image", false},
		{"dashscope", "video", false},
		{"dashscope", "tts", false},
		{"kling", "video", false},
		{"kling", "image", true},
		{"kling", "music", true},
		{"openai", "music", true},
		{"openai", "video", true},
	}
	for _, c := range cases {
		if got := adapterFor(c.provider, c.taskType); (got == nil) != c.wantNil {
			t.Errorf("adapterFor(%q, %q) nil = %v, want %v", c.provider, c.taskType, got == nil, c.wantNil)
		}
	}
}
