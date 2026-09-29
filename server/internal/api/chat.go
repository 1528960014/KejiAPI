package api

import (
	"bytes"
	"context"
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
// upstream channel for the requested model.
func (s *Server) handleChatCompletions(c *gin.Context) {
	ctx := c.Request.Context()
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

	model, err := s.store.GetModel(ctx, modelID)
	if errors.Is(err, store.ErrNotFound) || (model != nil && !model.Enabled) {
		abortWith(c, http.StatusNotFound, "model_not_found", "model not found or disabled: "+modelID)
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}

	ch, err := s.store.PickChannel(ctx, modelID)
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusBadGateway, "no_channel", "no enabled channel for model "+modelID)
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}

	upstreamBody, err := gateway.ReplaceModel(body, model.UpstreamModel)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	var reqMeta struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &reqMeta)

	res, err := s.provider.Chat(ctx, ch, upstreamBody)
	if err != nil {
		abortWith(c, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	defer res.Close()

	apiKey := s.apiKeyFrom(c)

	if res.StatusCode >= 400 {
		errBody, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		s.logUsage(ctx, apiKey, model, reqMeta.Stream, 0, 0, "upstream_error", string(errBody), 0)
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
		flusher, _ := c.Writer.(http.Flusher)
		buf := make([]byte, 32*1024)
		for {
			n, rerr := res.Body.Read(buf)
			if n > 0 {
				if _, werr := c.Writer.Write(buf[:n]); werr != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				slog.Warn("stream copy", "error", rerr)
				break
			}
		}
		s.logUsage(ctx, apiKey, model, true, 0, 0, "ok", "", 0)
		return
	}

	var out bytes.Buffer
	if _, err := io.Copy(c.Writer, io.TeeReader(res.Body, &out)); err != nil {
		return
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}

	var parsed struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(out.Bytes(), &parsed)
	cost := billing.Cost(model, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens)
	s.logUsage(ctx, apiKey, model, false, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, "ok", "", cost)
}

func (s *Server) logUsage(ctx context.Context, key *store.APIKey, m *store.Model, stream bool, prompt, completion int, status, errMsg string, cost float64) {
	u := &store.UsageLog{
		ModelID:          m.ModelID,
		Provider:         m.Provider,
		Stream:           stream,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		Cost:             cost,
		Status:           status,
		ErrorMsg:         errMsg,
	}
	if key != nil {
		u.APIKeyID = key.ID
	}
	logCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.store.LogUsage(logCtx, u); err != nil {
		slog.Warn("log usage", "error", err)
	}
}