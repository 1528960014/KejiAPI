package api

import (
	"strconv"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"modelhub/internal/store"
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
	c.JSON(http.StatusOK, gin.H{"data": channels})
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

func (s *Server) handleListKeys(c *gin.Context) {
	keys, err := s.store.ListAPIKeys(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": keys})
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

func parseIDParam(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_id", "id must be an integer")
		return 0, err
	}
	return id, nil
}
