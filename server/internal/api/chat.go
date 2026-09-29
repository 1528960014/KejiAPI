package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"modelhub/internal/billing"
	"modelhub/internal/gateway"
	"modelhub/internal/store"
)

// handleChatCompletions proxies an OpenAI-compatible chat request to the
// upstream channel for the requested model, with M2 billing:
// estimate cost -> enforce quota -> hold funds -> forward -> settle/release.
func (s *Server) handleChatCompletions(c *gin.Context) {
	ctx := c.Request.Context()
	key := s.apiKeyFrom(c)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "cannot read body")
		return
	}

	modelID, err := gateway.PeekModel(body)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// P2-2: the requested model may be a chat agent template; when it is,
	// inject its system prompt and route through the bound real model.
	var assistant *store.Assistant
	assistant, err = s.store.GetAssistantByID(ctx, modelID)
	if errors.Is(err, store.ErrNotFound) {
		assistant = nil
		err = nil
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	if assistant != nil && !assistant.Enabled {
		abortWith(c, http.StatusNotFound, "model_not_found", "agent not found or disabled: "+modelID)
		return
	}
	if assistant != nil {
		body, err = injectSystemPrompt(body, assistant.SystemPrompt)
		if err != nil {
			abortWith(c, http.StatusBadRequest, "invalid_request", "cannot parse request body")
			return
		}
		// P3-1: attach the template's tool schemas unless the request
		// already carries its own tools (the client's tools win).
		if len(assistant.Tools) > 0 {
			body, err = injectTools(body, assistant.Tools)
			if err != nil {
				abortWith(c, http.StatusBadRequest, "invalid_request", "cannot parse request body")
				return
			}
		}
	}

	targetModel := modelID
	if assistant != nil {
		targetModel = assistant.ModelID
	}
	model, err := s.store.GetModel(ctx, targetModel)
	if errors.Is(err, store.ErrNotFound) || (model != nil && !model.Enabled) {
		abortWith(c, http.StatusNotFound, "model_not_found", "model not found or disabled: "+targetModel)
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}

	// An agent is allowed when the key allowlist covers the agent id or the
	// bound real model; an empty allowlist allows everything.
	if !modelAllowed(key, modelID) && !modelAllowed(key, model.ModelID) {
		abortWith(c, http.StatusForbidden, "model_not_allowed", "API key is not allowed to use model "+modelID)
		return
	}

	ch, err := s.store.PickChannel(ctx, model.ModelID)
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusBadGateway, "no_channel", "no enabled channel for model "+modelID)
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}

	var reqMeta struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &reqMeta)

	// M2: estimate the cost, enforce the key quota, hold the funds.
	// P2-3: when the billing user is a reseller agent, resolve their
	// wholesale rate first and discount the estimate (and every settle).
	// P3-3: org keys bill the org's shared wallet at list price.
	promptEst, completionEst := billing.EstimateTokens(body)
	estMicro := billing.CostMicro(model, promptEst, completionEst)
	reqID := newRequestID()
	reason := "chat:" + modelID
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

	upstreamBody, err := gateway.PrepareUpstreamBody(body, model.UpstreamModel, reqMeta.Stream)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	res, err := s.provider.Chat(ctx, ch, upstreamBody)
	if err != nil {
		release()
		s.logUsage(ctx, key, model, reqMeta.Stream, 0, 0, "upstream_error", err.Error(), 0)
		abortWith(c, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	defer res.Close()

	if res.StatusCode >= 400 {
		errBody, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		release()
		s.logUsage(ctx, key, model, reqMeta.Stream, 0, 0, "upstream_error", string(errBody), 0)
		ct := res.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		c.Data(res.StatusCode, ct, errBody)
		return
	}

	ct := res.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	c.Header("Content-Type", ct)
	c.Status(http.StatusOK)

	if reqMeta.Stream {
		prompt, completion, gotUsage, writeErr := s.forwardStream(c, res.Body)
		if writeErr {
			release()
			s.logUsage(ctx, key, model, true, 0, 0, "client_gone", "client disconnected mid-stream", 0)
			return
		}
		status := "ok"
		actualMicro := estMicro
		if gotUsage {
			actualMicro = billing.ApplyRate(billing.CostMicro(model, prompt, completion), agentRate)
		} else {
			status = "ok_estimated"
			prompt, completion = 0, 0
		}
		settle(actualMicro)
		s.logUsage(ctx, key, model, true, prompt, completion, status, "", billing.USD(actualMicro))
		return
	}

	var out bytes.Buffer
	if _, err := io.Copy(c.Writer, io.TeeReader(res.Body, &out)); err != nil {
		settle(estMicro)
		s.logUsage(ctx, key, model, false, 0, 0, "client_gone", "client disconnected mid-response", 0)
		return
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}

	var parsed struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(out.Bytes(), &parsed)

	status := "ok"
	actualMicro := estMicro
	prompt, completion := 0, 0
	if parsed.Usage != nil && (parsed.Usage.PromptTokens > 0 || parsed.Usage.CompletionTokens > 0) {
		prompt, completion = parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens
		actualMicro = billing.ApplyRate(billing.CostMicro(model, prompt, completion), agentRate)
	} else {
		status = "ok_estimated"
	}
	settle(actualMicro)
	s.logUsage(ctx, key, model, false, prompt, completion, status, "", billing.USD(actualMicro))
}

// keyUserID returns the billing user bound to the key, if any.
func keyUserID(key *store.APIKey) *int64 {
	if key == nil {
		return nil
	}
	return key.UserID
}

// modelAllowed reports whether the key may call the given model; an empty
// allowlist means all models.
func modelAllowed(key *store.APIKey, modelID string) bool {
	if key == nil || len(key.AllowedModels) == 0 {
		return true
	}
	for _, m := range key.AllowedModels {
		if m == modelID {
			return true
		}
	}
	return false
}

// injectSystemPrompt prepends a system message to the request body's message
// list and returns the re-serialized body.
func injectSystemPrompt(body []byte, system string) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	msgs, _ := payload["messages"].([]any)
	next := make([]any, 0, len(msgs)+1)
	next = append(next, map[string]any{"role": "system", "content": system})
	next = append(next, msgs...)
	payload["messages"] = next
	return json.Marshal(payload)
}

// injectTools adds the template's OpenAI-format tools array to the request
// body. A request that already carries its own tools is returned unchanged.
func injectTools(body []byte, tools []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if existing, ok := payload["tools"]; ok && existing != nil {
		return body, nil
	}
	var tv any
	if err := json.Unmarshal(tools, &tv); err != nil {
		return nil, err
	}
	payload["tools"] = tv
	return json.Marshal(payload)
}

// newRequestID builds a unique request ID used to tag ledger entries.
func newRequestID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "req-" + hex.EncodeToString(buf)
}

// forwardStream copies the upstream SSE body to the client verbatim, line by
// line, and captures token usage from a data chunk when the upstream honors
// stream_options.include_usage.
func (s *Server) forwardStream(c *gin.Context, body io.Reader) (prompt, completion int, gotUsage, writeErr bool) {
	flusher, _ := c.Writer.(http.Flusher)
	r := bufio.NewReaderSize(body, 64*1024)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if bytes.HasPrefix(line, []byte("data:")) && bytes.Contains(line, []byte(`"usage"`)) {
				var chunk struct {
					Usage *struct {
						PromptTokens     int `json:"prompt_tokens"`
						CompletionTokens int `json:"completion_tokens"`
					} `json:"usage"`
				}
				if jerr := json.Unmarshal(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:"))), &chunk); jerr == nil && chunk.Usage != nil {
					prompt, completion = chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
					gotUsage = true
				}
			}
			if _, werr := c.Writer.Write(line); werr != nil {
				writeErr = true
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Warn("stream copy", "error", err)
			writeErr = true
			break
		}
	}
	return
}

func (s *Server) logUsage(ctx context.Context, key *store.APIKey, m *store.Model, stream bool, prompt, completion int, status, errMsg string, costUSD float64) {
	u := &store.UsageLog{
		ModelID:          m.ModelID,
		Provider:         m.Provider,
		Stream:           stream,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		Cost:             costUSD,
		Status:           status,
		ErrorMsg:         errMsg,
	}
	if key != nil {
		u.APIKeyID = &key.ID
	}
	logCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.store.LogUsage(logCtx, u); err != nil {
		slog.Warn("log usage", "error", err)
	}
}
