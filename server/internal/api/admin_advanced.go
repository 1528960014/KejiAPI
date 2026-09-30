package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"kejiapi/internal/billing"
	"kejiapi/internal/store"
)

// This file holds the "second-generation" admin endpoints: partial updates
// (patch), the live channel test, and the daily usage aggregate for the
// dashboard chart.

// --- channels: update + live test ---

type updateChannelReq struct {
	Name     *string `json:"name"`
	BaseURL  *string `json:"base_url"`
	APIKey   *string `json:"api_key"`
	ModelID  *string `json:"model_id"`
	Priority *int    `json:"priority"`
	Enabled  *bool   `json:"enabled"`
}

func (s *Server) handleUpdateChannel(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req updateChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.Enabled == nil && req.Name == nil && req.BaseURL == nil && req.APIKey == nil &&
		req.ModelID == nil && req.Priority == nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "nothing to update")
		return
	}
	if req.Name != nil && *req.Name == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "name cannot be empty")
		return
	}
	if req.ModelID != nil {
		if _, mErr := s.store.GetModel(c.Request.Context(), *req.ModelID); errors.Is(mErr, store.ErrNotFound) {
			abortWith(c, http.StatusBadRequest, "unknown_model", "model_id does not exist")
			return
		}
	}
	ch, err := s.store.UpdateChannel(c.Request.Context(), id, &store.ChannelPatch{
		Name: req.Name, BaseURL: req.BaseURL, APIKey: req.APIKey,
		ModelID: req.ModelID, Priority: req.Priority, Enabled: req.Enabled,
	})
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	now := time.Now()
	item := channelWithHealth{Channel: *ch, Health: "ok"}
	if until, down := s.health.CooldownUntil(ch.ID, now); down {
		item.Health = "cooldown"
		item.CooldownUntil = until.UTC().Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, item)
}

// handleTestChannel runs a minimal non-streaming completion (max_tokens=1)
// through one channel and reports the upstream status plus latency, so the
// admin can verify credentials, base URL and model availability at a glance.
func (s *Server) handleTestChannel(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	ctx := c.Request.Context()
	ch, err := s.store.GetChannel(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	model, err := s.store.GetModel(ctx, ch.ModelID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "model " + ch.ModelID + " not configured"})
		return
	}
	body, _ := json.Marshal(map[string]any{
		"model":      model.UpstreamModel,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
		"stream":     false,
	})
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	start := time.Now()
	status, raw, derr := s.provider.DoJSON(tctx, ch, http.MethodPost, "/chat/completions", body, nil)
	latency := time.Since(start).Milliseconds()
	resp := gin.H{"ok": status >= 200 && status < 300, "status": status, "latency_ms": latency}
	if derr != nil {
		resp["error"] = derr.Error()
	} else if status >= 300 {
		msg := string(raw)
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		resp["error"] = msg
	}
	c.JSON(http.StatusOK, resp)
}

// --- models: update ---

type updateModelReq struct {
	UpstreamModel    *string   `json:"upstream_model"`
	Capabilities     *[]string `json:"capabilities"`
	InputPricePer1k  *float64  `json:"input_price_per_1k"`
	OutputPricePer1k *float64  `json:"output_price_per_1k"`
	PriceUnit        *string   `json:"price_unit"`
	UnitPrice        *float64  `json:"unit_price"`
	Enabled          *bool     `json:"enabled"`
}

func (s *Server) handleUpdateModel(c *gin.Context) {
	modelID := c.Param("id")
	ctx := c.Request.Context()
	m, err := s.store.GetModel(ctx, modelID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	var req updateModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.UpstreamModel != nil {
		m.UpstreamModel = *req.UpstreamModel
	}
	if req.Capabilities != nil {
		m.Capabilities = *req.Capabilities
	}
	if req.InputPricePer1k != nil {
		m.InputPricePer1k = *req.InputPricePer1k
	}
	if req.OutputPricePer1k != nil {
		m.OutputPricePer1k = *req.OutputPricePer1k
	}
	if req.PriceUnit != nil {
		m.PriceUnit = *req.PriceUnit
	}
	if req.UnitPrice != nil {
		m.UnitPrice = *req.UnitPrice
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}
	if err := s.store.CreateModel(ctx, m); err != nil { // upsert
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// --- users: enable/disable ---

type updateUserReq struct {
	Enabled *bool `json:"enabled"`
}

func (s *Server) handleUpdateUser(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req updateUserReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "enabled (bool) is required")
		return
	}
	u, err := s.store.SetUserEnabled(c.Request.Context(), id, *req.Enabled)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, userJSON(u))
}

// --- usage: daily aggregate for the dashboard chart ---

func (s *Server) handleUsageDaily(c *gin.Context) {
	days := 14
	if v, err := strconv.Atoi(c.Query("days")); err == nil {
		days = v
	}
	if days < 1 {
		days = 14
	}
	if days > 90 {
		days = 90
	}
	rows, err := s.store.UsageDaily(c.Request.Context(), days)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(rows))
	for _, d := range rows {
		data = append(data, gin.H{
			"date":              d.Date,
			"requests":          d.Requests,
			"prompt_tokens":     d.PromptTokens,
			"completion_tokens": d.CompletionTokens,
			"cost_micro":        d.CostMicro,
			"cost_usd":          billing.USD(d.CostMicro),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}
