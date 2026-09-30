package task

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"kejiapi/internal/gateway"
	"kejiapi/internal/store"
)

// Adapter implements one upstream protocol for one media type. Run blocks
// until the generation finishes (synchronous upstreams return immediately;
// asynchronous upstreams poll internally) and returns the result URLs.
type Adapter interface {
	Run(ctx context.Context, p *gateway.Provider, ch *store.Channel, m *store.Model, payload []byte) ([]string, error)
}

// pollInterval is how often async upstreams are re-checked (var so tests can
// speed it up).
var pollInterval = 5 * time.Second

// adapterFor picks the protocol implementation for a provider + media type.
// nil means the combination is not supported yet; the worker fails the task
// with a clear message instead of guessing.
func adapterFor(provider, taskType string) Adapter {
	switch taskType {
	case "image":
		// OpenAI-compatible /images/generations is the common BYO shape
		// (OpenAI, SiliconFlow, DashScope compatible mode, self-hosted
		// gateways, ...). Kling's image API is native async, not this shape.
		if provider == "kling" {
			return nil
		}
		return openaiImageAdapter{}
	case "video":
		switch provider {
		case "dashscope":
			return dashscopeVideoAdapter{}
		case "kling":
			return klingVideoAdapter{}
		}
	case "tts":
		if provider == "dashscope" {
			return dashscopeTTSAdapter{}
		}
	case "music":
		// Suno's API (community/BYO key, api.suno.com): submit + poll for a
		// finished audio_url. Other providers have no standard music API.
		if provider == "suno" {
			return sunoMusicAdapter{}
		}
	}
	return nil
}

// --- shared payload handling ---

type mediaPayload struct {
	Prompt   string `json:"prompt"`
	Text     string `json:"text"`
	N        int    `json:"n"`
	Size     string `json:"size"`
	Duration string `json:"duration"`
}

func parsePayload(raw []byte) (*mediaPayload, error) {
	var p mediaPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid task payload: %w", err)
	}
	if p.Prompt == "" && p.Text == "" {
		return nil, fmt.Errorf("payload requires prompt or text")
	}
	return &p, nil
}

func trimBody(b []byte) string {
	const max = 300
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// --- OpenAI-compatible synchronous image generation ---

type openaiImageAdapter struct{}

func (openaiImageAdapter) Run(ctx context.Context, p *gateway.Provider, ch *store.Channel, m *store.Model, payload []byte) ([]string, error) {
	parsed, err := parsePayload(payload)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	n := parsed.N
	if n <= 0 {
		n = 1
	}
	size := parsed.Size
	if size == "" {
		size = "1024x1024"
	}
	body, _ := json.Marshal(map[string]any{
		"model":  m.UpstreamModel,
		"prompt": parsed.Prompt,
		"n":      n,
		"size":   size,
	})
	status, respBody, err := p.DoJSON(ctx, ch, http.MethodPost, "/images/generations", body, nil)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	if status >= 400 {
		return nil, &UpstreamHTTPError{Status: status, Body: respBody}
	}
	var out struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	urls := make([]string, 0, len(out.Data))
	for _, d := range out.Data {
		if d.URL != "" {
			urls = append(urls, d.URL)
			continue
		}
		if d.B64JSON != "" {
			return nil, fmt.Errorf("upstream returned b64_json, but object storage for persisting it is not configured; use a channel that returns image urls")
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("upstream returned no image urls")
	}
	return urls, nil
}

// --- DashScope native async video synthesis ---

type dashscopeVideoAdapter struct{}

func (dashscopeVideoAdapter) Run(ctx context.Context, p *gateway.Provider, ch *store.Channel, m *store.Model, payload []byte) ([]string, error) {
	parsed, err := parsePayload(payload)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	body := map[string]any{
		"model": m.UpstreamModel,
		"input": map[string]any{"prompt": parsed.Prompt},
	}
	if parsed.Duration != "" {
		body["parameters"] = map[string]any{"duration": parsed.Duration}
	}
	raw, _ := json.Marshal(body)
	status, respBody, err := p.DoJSON(ctx, ch, http.MethodPost,
		"/api/v1/services/aigc/video-generation/video-synthesis", raw,
		map[string]string{"X-DashScope-Async": "enable"})
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	if status >= 400 {
		return nil, &UpstreamHTTPError{Status: status, Body: respBody}
	}
	var sub struct {
		Output struct {
			TaskID     string `json:"task_id"`
			TaskStatus string `json:"task_status"`
			Message    string `json:"message"`
		} `json:"output"`
	}
	if err := json.Unmarshal(respBody, &sub); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	if sub.Output.TaskID == "" {
		return nil, fmt.Errorf("upstream did not return a task_id: %s", sub.Output.Message)
	}
	return pollDashscopeTask(ctx, p, ch, sub.Output.TaskID)
}

func pollDashscopeTask(ctx context.Context, p *gateway.Provider, ch *store.Channel, taskID string) ([]string, error) {
	for {
		status, respBody, err := p.DoJSON(ctx, ch, http.MethodGet, "/api/v1/tasks/"+taskID, nil, nil)
		if err != nil {
			return nil, &TransportError{Err: err}
		}
		if status >= 400 {
			return nil, &UpstreamHTTPError{Status: status, Body: respBody}
		}
		var out struct {
			Output struct {
				TaskStatus string `json:"task_status"`
				VideoURL   string `json:"video_url"`
				Results    []struct {
					URL string `json:"url"`
				} `json:"results"`
				Message string `json:"message"`
			} `json:"output"`
		}
		if err := json.Unmarshal(respBody, &out); err != nil {
			return nil, fmt.Errorf("parse upstream response: %w", err)
		}
		switch out.Output.TaskStatus {
		case "SUCCEEDED":
			urls := make([]string, 0, 1+len(out.Output.Results))
			if out.Output.VideoURL != "" {
				urls = append(urls, out.Output.VideoURL)
			}
			for _, r := range out.Output.Results {
				if r.URL != "" {
					urls = append(urls, r.URL)
				}
			}
			if len(urls) == 0 {
				return nil, fmt.Errorf("upstream succeeded but returned no result urls")
			}
			return urls, nil
		case "FAILED", "CANCELED", "UNKNOWN":
			return nil, fmt.Errorf("upstream task %s: %s", out.Output.TaskStatus, out.Output.Message)
		}
		if err := sleepCtx(ctx, pollInterval); err != nil {
			return nil, err
		}
	}
}

// --- Kling async text2video ---

type klingVideoAdapter struct{}

func (klingVideoAdapter) Run(ctx context.Context, p *gateway.Provider, ch *store.Channel, m *store.Model, payload []byte) ([]string, error) {
	parsed, err := parsePayload(payload)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	body := map[string]any{"model": m.UpstreamModel, "prompt": parsed.Prompt}
	if parsed.Duration != "" {
		body["duration"] = parsed.Duration
	}
	raw, _ := json.Marshal(body)
	status, respBody, err := p.DoJSON(ctx, ch, http.MethodPost, "/v1/videos/text2video", raw, nil)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	if status >= 400 {
		return nil, &UpstreamHTTPError{Status: status, Body: respBody}
	}
	var sub struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			TaskInfo struct {
				ID string `json:"id"`
			} `json:"task_info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &sub); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	if sub.Data.TaskInfo.ID == "" {
		return nil, fmt.Errorf("upstream did not accept the task (code %d): %s", sub.Code, sub.Message)
	}
	return pollKlingTask(ctx, p, ch, sub.Data.TaskInfo.ID)
}

func pollKlingTask(ctx context.Context, p *gateway.Provider, ch *store.Channel, taskID string) ([]string, error) {
	for {
		status, respBody, err := p.DoJSON(ctx, ch, http.MethodGet, "/v1/videos/text2video/"+taskID, nil, nil)
		if err != nil {
			return nil, err
		}
		if status >= 400 {
			return nil, &UpstreamHTTPError{Status: status, Body: respBody}
		}
		var out struct {
			Data struct {
				TaskInfo struct {
					TaskStatus string `json:"task_status"`
					FailReason string `json:"fail_reason"`
					TaskResult struct {
						Videos []struct {
							URL string `json:"url"`
						} `json:"videos"`
					} `json:"task_result"`
				} `json:"task_info"`
			} `json:"data"`
		}
		if err := json.Unmarshal(respBody, &out); err != nil {
			return nil, fmt.Errorf("parse upstream response: %w", err)
		}
		info := out.Data.TaskInfo
		switch info.TaskStatus {
		case "succeeded":
			urls := make([]string, 0, len(info.TaskResult.Videos))
			for _, v := range info.TaskResult.Videos {
				if v.URL != "" {
					urls = append(urls, v.URL)
				}
			}
			if len(urls) == 0 {
				return nil, fmt.Errorf("upstream succeeded but returned no video urls")
			}
			return urls, nil
		case "failed":
			return nil, fmt.Errorf("upstream task failed: %s", info.FailReason)
		}
		if err := sleepCtx(ctx, pollInterval); err != nil {
			return nil, err
		}
	}
}

// --- Suno async text-to-music (community API, BYO key) ---

type sunoMusicAdapter struct{}

func (sunoMusicAdapter) Run(ctx context.Context, p *gateway.Provider, ch *store.Channel, m *store.Model, payload []byte) ([]string, error) {
	parsed, err := parsePayload(payload)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	text := parsed.Prompt
	if text == "" {
		text = parsed.Text
	}
	body := map[string]any{"text": text}
	if m.UpstreamModel != "" {
		body["model"] = m.UpstreamModel
	}
	raw, _ := json.Marshal(body)
	status, respBody, err := p.DoJSON(ctx, ch, http.MethodPost, "/api/v1/generate", raw, nil)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	if status >= 400 {
		return nil, &UpstreamHTTPError{Status: status, Body: respBody}
	}
	var sub struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &sub); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	if sub.ID == "" {
		return nil, fmt.Errorf("upstream did not return a task id: %s", trimBody(respBody))
	}
	return pollSunoTask(ctx, p, ch, sub.ID)
}

func pollSunoTask(ctx context.Context, p *gateway.Provider, ch *store.Channel, taskID string) ([]string, error) {
	for {
		status, respBody, err := p.DoJSON(ctx, ch, http.MethodGet, "/api/v1/generate/"+taskID, nil, nil)
		if err != nil {
			return nil, err
		}
		if status >= 400 {
			return nil, &UpstreamHTTPError{Status: status, Body: respBody}
		}
		var out struct {
			Status     string `json:"status"`
			AudioURL   string `json:"audio_url"`
			FailReason string `json:"fail_reason"`
		}
		if err := json.Unmarshal(respBody, &out); err != nil {
			return nil, fmt.Errorf("parse upstream response: %w", err)
		}
		switch out.Status {
		case "complete", "succeeded":
			if out.AudioURL == "" {
				return nil, fmt.Errorf("upstream succeeded but returned no audio url")
			}
			return []string{out.AudioURL}, nil
		case "failed", "canceled", "unknown":
			return nil, fmt.Errorf("upstream task failed: %s", out.FailReason)
		}
		if err := sleepCtx(ctx, pollInterval); err != nil {
			return nil, err
		}
	}
}

// --- DashScope synchronous TTS (CosyVoice) ---

type dashscopeTTSAdapter struct{}

func (dashscopeTTSAdapter) Run(ctx context.Context, p *gateway.Provider, ch *store.Channel, m *store.Model, payload []byte) ([]string, error) {
	parsed, err := parsePayload(payload)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	text := parsed.Text
	if text == "" {
		text = parsed.Prompt
	}
	body, _ := json.Marshal(map[string]any{
		"model": m.UpstreamModel,
		"input": map[string]any{"text": text},
	})
	status, respBody, err := p.DoJSON(ctx, ch, http.MethodPost,
		"/api/v1/services/aigc/multimodal-generation/generation", body, nil)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	if status >= 400 {
		return nil, &UpstreamHTTPError{Status: status, Body: respBody}
	}
	var out struct {
		Output struct {
			Audio struct {
				URL string `json:"url"`
			} `json:"audio"`
		} `json:"output"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	if out.Output.Audio.URL != "" {
		return []string{out.Output.Audio.URL}, nil
	}
	return nil, fmt.Errorf("upstream tts error: %s %s", out.Code, out.Message)
}
