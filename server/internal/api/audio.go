package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"modelhub/internal/billing"
	"modelhub/internal/store"
)

// P3-4: real-time speech — an OpenAI-compatible synchronous text-to-speech
// endpoint. The finished audio is returned in the HTTP response itself (no
// task queue, no polling), so clients can start playing immediately:
//
//	POST /v1/audio/speech
//	{"model": "tts-1", "input": "hello", "voice": "alloy",
//	 "response_format": "mp3", "speed": 1.0}
//
// The model must carry the "tts" capability. Upstreams: OpenAI-compatible
// /audio/speech (binary reply) and DashScope CosyVoice (URL reply, fetched
// and relayed). Billing: the model's unit_price, held before the call and
// settled on success / released on failure (org keys bill the org wallet at
// list price, like every other endpoint).

// maxSpeechInputChars caps one synthesis (OpenAI's tts-1 limit is 4096).
const maxSpeechInputChars = 4096

// speechFormats maps OpenAI response_format values to the client-facing
// Content-Type.
var speechFormats = map[string]string{
	"mp3":  "audio/mpeg",
	"opus": "audio/ogg",
	"aac":  "audio/aac",
	"flac": "audio/flac",
	"wav":  "audio/wav",
	"pcm":  "audio/L16",
}

func speechContentType(format string) (string, bool) {
	ct, ok := speechFormats[format]
	return ct, ok
}

type speechRequest struct {
	Model          string   `json:"model"`
	Input          string   `json:"input"`
	Voice          string   `json:"voice"`
	ResponseFormat string   `json:"response_format"`
	Speed          *float64 `json:"speed"`
}

// speechUpstreamBody builds the OpenAI-compatible /audio/speech body with the
// upstream model name substituted.
func speechUpstreamBody(m *store.Model, req *speechRequest) []byte {
	format := req.ResponseFormat
	if format == "" {
		format = "mp3"
	}
	body := map[string]any{
		"model":           m.UpstreamModel,
		"input":           req.Input,
		"voice":           req.Voice,
		"response_format": format,
	}
	if req.Speed != nil {
		body["speed"] = *req.Speed
	}
	raw, _ := json.Marshal(body)
	return raw
}

func (s *Server) handleAudioSpeech(c *gin.Context) {
	ctx := c.Request.Context()
	key := s.apiKeyFrom(c)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "cannot read body")
		return
	}
	var req speechRequest
	if err := json.Unmarshal(body, &req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.Model == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "model is required")
		return
	}
	if req.Input == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "input is required")
		return
	}
	if utf8.RuneCountInString(req.Input) > maxSpeechInputChars {
		abortWith(c, http.StatusBadRequest, "input_too_long", fmt.Sprintf("input exceeds %d characters", maxSpeechInputChars))
		return
	}
	format := req.ResponseFormat
	if format == "" {
		format = "mp3"
	}
	contentType, ok := speechContentType(format)
	if !ok {
		abortWith(c, http.StatusBadRequest, "invalid_format", "response_format must be one of mp3/opus/aac/flac/wav/pcm")
		return
	}

	model, err := s.store.GetModel(ctx, req.Model)
	if errors.Is(err, store.ErrNotFound) || (model != nil && !model.Enabled) {
		abortWith(c, http.StatusNotFound, "model_not_found", "model not found or disabled: "+req.Model)
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	if !hasCapability(model.Capabilities, "tts") {
		abortWith(c, http.StatusNotFound, "model_not_found", "model "+req.Model+" does not support tts")
		return
	}
	if !modelAllowed(key, req.Model) {
		abortWith(c, http.StatusForbidden, "model_not_allowed", "API key is not allowed to use model "+req.Model)
		return
	}

	if key != nil && s.limiter.Enabled() {
		if ok, which := s.limiter.AllowRequest(key.ID); !ok {
			abortWith(c, http.StatusTooManyRequests, "rate_limited", rateLimitMessage(which))
			return
		}
	}

	ch, err := s.store.PickChannel(ctx, model.ModelID)
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusBadGateway, "no_channel", "no enabled channel for model "+req.Model)
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}

	// M2 billing: the TTS unit price, held up front (P2-3 agent wholesale,
	// P3-3 org wallet at list price).
	estMicro := billing.MediaCostMicro(model, 1)
	if estMicro == 0 && model.PriceUnit == "token" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "model "+req.Model+" has no media price configured (set unit_price)")
		return
	}
	reqID := newRequestID()
	reason := "speech:" + req.Model
	held := false
	billOrg := key != nil && key.OrgID != nil
	var agentRate *float64
	if !billOrg {
		if userID := keyUserID(key); userID != nil && estMicro > 0 {
			user, err := s.store.GetUser(ctx, *userID)
			if errors.Is(err, store.ErrNotFound) {
				abortWith(c, http.StatusForbidden, "user_not_found", "billing user of this API key no longer exists")
				return
			}
			if err != nil {
				httpErr(c, err)
				return
			}
			if !user.Enabled {
				abortWith(c, http.StatusForbidden, "user_disabled", "billing user is disabled")
				return
			}
			agentRate = user.AgentRate
			estMicro = billing.ApplyRate(estMicro, agentRate)
		}
	}
	if key != nil && key.Quota != nil && key.Spend+estMicro > *key.Quota {
		abortWith(c, http.StatusTooManyRequests, "quota_exceeded", "API key quota exhausted; ask the admin to raise the quota")
		return
	}
	if billOrg && estMicro > 0 {
		if err := s.store.HoldOrgFunds(ctx, *key.OrgID, estMicro, reqID, reason); err != nil {
			if errors.Is(err, store.ErrInsufficientBalance) {
				abortWith(c, http.StatusPaymentRequired, "insufficient_balance", "insufficient organization balance; top up the org via the admin API")
				return
			}
			httpErr(c, err)
			return
		}
		held = true
	} else if userID := keyUserID(key); userID != nil && estMicro > 0 {
		if err := s.store.HoldFunds(ctx, *userID, estMicro, reqID, reason); err != nil {
			if errors.Is(err, store.ErrInsufficientBalance) {
				abortWith(c, http.StatusPaymentRequired, "insufficient_balance", "insufficient balance; top up this user via the admin API")
				return
			}
			httpErr(c, err)
			return
		}
		held = true
	}

	settle := func(actualMicro int64) {
		if key == nil {
			return
		}
		holdMicro := int64(0)
		if held {
			holdMicro = estMicro
		}
		sc, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		var err error
		if billOrg {
			err = s.store.SettleOrgFunds(sc, key.OrgID, holdMicro, actualMicro, reqID, reason, &key.ID)
		} else {
			err = s.store.SettleFunds(sc, keyUserID(key), holdMicro, actualMicro, reqID, reason, &key.ID)
		}
		if err != nil {
			slog.Warn("settle funds", "error", err, "request_id", reqID)
		}
	}
	release := func() {
		if !held {
			return
		}
		sc, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		var err error
		if billOrg {
			err = s.store.ReleaseOrgFunds(sc, *key.OrgID, estMicro, reqID, reason)
		} else {
			err = s.store.ReleaseFunds(sc, *keyUserID(key), estMicro, reqID, reason)
		}
		if err != nil {
			slog.Warn("release funds", "error", err, "request_id", reqID)
		}
	}

	fail := func(status int, code, msg string) {
		release()
		s.logUsage(ctx, key, model, false, 0, 0, "upstream_error", msg, 0)
		abortWith(c, status, code, msg)
	}

	var audio []byte
	if model.Provider == "dashscope" {
		audio, err = s.synthesizeDashscopeSpeech(ctx, ch, model, req.Input)
	} else {
		rawBody := speechUpstreamBody(model, &req)
		var status int
		status, audio, err = s.provider.DoJSON(ctx, ch, http.MethodPost, "/audio/speech", rawBody, nil)
		if err == nil && status >= 400 {
			err = fmt.Errorf("upstream returned HTTP %d: %s", status, trimErrBody(audio))
			audio = nil
		}
	}
	if err != nil {
		fail(http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if len(audio) == 0 {
		fail(http.StatusBadGateway, "upstream_error", "upstream returned no audio")
		return
	}

	settle(estMicro)
	s.logUsage(ctx, key, model, false, 0, 0, "ok", "", billing.USD(estMicro))
	c.Data(http.StatusOK, contentType, audio)
}

// synthesizeDashscopeSpeech calls DashScope CosyVoice (URL reply) and
// downloads the audio so the endpoint can always return bytes.
func (s *Server) synthesizeDashscopeSpeech(ctx context.Context, ch *store.Channel, m *store.Model, input string) ([]byte, error) {
	body, _ := json.Marshal(map[string]any{
		"model": m.UpstreamModel,
		"input": map[string]any{"text": input},
	})
	status, respBody, err := s.provider.DoJSON(ctx, ch, http.MethodPost,
		"/api/v1/services/aigc/multimodal-generation/generation", body, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("upstream returned HTTP %d: %s", status, trimErrBody(respBody))
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
	if out.Output.Audio.URL == "" {
		return nil, fmt.Errorf("upstream tts error: %s %s", out.Code, out.Message)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, out.Output.Audio.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.provider.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch audio: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch audio: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
}

func trimErrBody(b []byte) string {
	const max = 300
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}
