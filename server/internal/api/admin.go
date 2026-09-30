package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"kejiapi/internal/billing"
	"kejiapi/internal/store"
)

// --- models ---

func (s *Server) handleListModels(c *gin.Context) {
	models, err := s.store.ListModels(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": models})
}

type createModelReq struct {
	ModelID          string   `json:"model_id"`
	Provider         string   `json:"provider"`
	UpstreamModel    string   `json:"upstream_model"`
	Capabilities     []string `json:"capabilities"`
	InputPricePer1k  float64  `json:"input_price_per_1k"`
	OutputPricePer1k float64  `json:"output_price_per_1k"`
	PriceUnit        string   `json:"price_unit"` // 'token' (default) | 'image' | 'video' | 'music' | 'tts'
	UnitPrice        float64  `json:"unit_price"` // USD per generated item when price_unit != 'token'
	Enabled          *bool    `json:"enabled"`
}

func (s *Server) handleCreateModel(c *gin.Context) {
	var req createModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.ModelID == "" || req.Provider == "" || req.UpstreamModel == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "model_id, provider and upstream_model are required")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	capabilities := req.Capabilities
	if capabilities == nil {
		capabilities = []string{}
	}
	err := s.store.CreateModel(c.Request.Context(), &store.Model{
		ModelID:          req.ModelID,
		Provider:         req.Provider,
		UpstreamModel:    req.UpstreamModel,
		Capabilities:     capabilities,
		InputPricePer1k:  req.InputPricePer1k,
		OutputPricePer1k: req.OutputPricePer1k,
		PriceUnit:        req.PriceUnit,
		UnitPrice:        req.UnitPrice,
		Enabled:          enabled,
	})
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleDeleteModel(c *gin.Context) {
	if err := s.store.DeleteModel(c.Request.Context(), c.Param("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// --- channels ---

func (s *Server) handleListChannels(c *gin.Context) {
	channels, err := s.store.ListChannels(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	// P5-1: annotate each channel with its in-memory failover health.
	now := time.Now()
	probes, _ := s.store.ListChannelHealth(c.Request.Context()) // best-effort
	out := make([]channelWithHealth, 0, len(channels))
	for _, ch := range channels {
		item := channelWithHealth{Channel: ch, Health: "ok", LastProbe: probes[ch.ID]}
		if until, down := s.health.CooldownUntil(ch.ID, now); down {
			item.Health = "cooldown"
			item.CooldownUntil = until.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// channelWithHealth is a channel plus its P5-1 failover health (cooldown
// state is per-instance in-memory, like the rate limiter).
type channelWithHealth struct {
	store.Channel
	Health        string                   `json:"health"`
	CooldownUntil string                   `json:"cooldown_until,omitempty"`
	LastProbe     *store.ChannelHealthRow  `json:"last_probe,omitempty"`
}

type createChannelReq struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	ModelID  string `json:"model_id"`
	Priority int    `json:"priority"`
}

func (s *Server) handleCreateChannel(c *gin.Context) {
	var req createChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.Name == "" || req.Provider == "" || req.BaseURL == "" || req.APIKey == "" || req.ModelID == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "name, provider, base_url, api_key and model_id are required")
		return
	}
	if err := s.store.CreateChannel(c.Request.Context(), &store.Channel{
		Name: req.Name, Provider: req.Provider, BaseURL: req.BaseURL,
		APIKey: req.APIKey, ModelID: req.ModelID, Priority: req.Priority, Enabled: true,
	}); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleDeleteChannel(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeleteChannel(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// --- api keys ---

// keyJSON renders an API key for the admin API; monetary fields in USD.
func keyJSON(k *store.APIKey) gin.H {
	item := gin.H{
		"id":             k.ID,
		"name":           k.Name,
		"allowed_models": k.AllowedModels,
		"spend_micro":    k.Spend,
		"spend_usd":      billing.USD(k.Spend),
		"markup":         k.Markup,
		"created_at":     k.CreatedAt,
	}
	if k.UserID != nil {
		item["user_id"] = *k.UserID
	}
	if k.AgentID != nil {
		item["agent_id"] = *k.AgentID
	}
	if k.OrgID != nil {
		item["org_id"] = *k.OrgID
	}
	if k.Quota != nil {
		item["quota_usd"] = billing.USD(*k.Quota)
	}
	if k.ExpiresAt != nil {
		item["expires_at"] = *k.ExpiresAt
	}
	return item
}

func (s *Server) handleListKeys(c *gin.Context) {
	keys, err := s.store.ListAPIKeys(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(keys))
	for i := range keys {
		data = append(data, keyJSON(&keys[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func (s *Server) handleCreateKey(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	plain, key, err := s.store.CreateAPIKey(c.Request.Context(), req.Name)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"key": plain, "id": key.ID, "name": key.Name})
}

func (s *Server) handleDeleteKey(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeleteAPIKey(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// parsePagination reads ?limit=&offset= with a default limit and hard cap.
func parsePagination(c *gin.Context, defLimit, maxLimit int) (int, int) {
	limit, offset := defLimit, 0
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = v
	}
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v > 0 {
		offset = v
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return limit, offset
}

func parseIDParam(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_id", "id must be an integer")
		return 0, err
	}
	return id, nil
}

type updateKeyReq struct {
	Name          *string   `json:"name"`
	UserID        *int64    `json:"user_id"`        // 0 unbinds the key from its user
	QuotaUSD      *float64  `json:"quota_usd"`      // 0 removes the quota
	AllowedModels *[]string `json:"allowed_models"` // empty slice = allow all
	ExpiresAt     *string   `json:"expires_at"`     // RFC3339; "" removes the expiry
}

// handleUpdateKey patches an existing API key (binding, quota, allowlist, expiry).
func (s *Server) handleUpdateKey(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req updateKeyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	patch := &store.KeyPatch{
		Name:          req.Name,
		UserID:        req.UserID,
		AllowedModels: req.AllowedModels,
	}
	if req.QuotaUSD != nil {
		if *req.QuotaUSD < 0 {
			abortWith(c, http.StatusBadRequest, "invalid_request", "quota_usd must be >= 0")
			return
		}
		q := int64(0)
		if *req.QuotaUSD > 0 {
			q = billing.ToMicro(*req.QuotaUSD)
		}
		patch.Quota = &q
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			zero := time.Time{}
			patch.ExpiresAt = &zero
		} else {
			t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
			if err != nil {
				abortWith(c, http.StatusBadRequest, "invalid_expires_at", "expires_at must be RFC3339 or empty")
				return
			}
			patch.ExpiresAt = &t
		}
	}
	key, err := s.store.UpdateAPIKey(c.Request.Context(), id, patch)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, keyJSON(key))
}
