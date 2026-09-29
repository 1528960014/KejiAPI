package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"modelhub/internal/store"
)

// assistantJSON renders the public form of a chat-agent template.
func assistantJSON(a *store.Assistant) gin.H {
	return gin.H{
		"agent_id":    a.AgentID,
		"name":        a.Name,
		"description": a.Description,
		"model":       a.ModelID,
	}
}

// assistantAdminJSON renders the full template for the admin API.
func assistantAdminJSON(a *store.Assistant) gin.H {
	item := gin.H{
		"id":            a.ID,
		"agent_id":      a.AgentID,
		"name":          a.Name,
		"description":   a.Description,
		"system_prompt": a.SystemPrompt,
		"model":         a.ModelID,
		"enabled":       a.Enabled,
		"created_at":    a.CreatedAt,
	}
	return item
}

// handleListChatAgents lists enabled templates for API-key clients.
func (s *Server) handleListChatAgents(c *gin.Context) {
	list, err := s.store.ListEnabledAssistants(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		data = append(data, assistantJSON(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleListAssistants lists all templates (admin).
func (s *Server) handleListAssistants(c *gin.Context) {
	list, err := s.store.ListAssistants(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		data = append(data, assistantAdminJSON(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

type assistantReq struct {
	AgentID      string `json:"agent_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	SystemPrompt string `json:"system_prompt"`
	Model        string `json:"model"`
	Enabled      *bool  `json:"enabled"`
}

// handleCreateAssistant creates a template (admin).
func (s *Server) handleCreateAssistant(c *gin.Context) {
	var req assistantReq
	if err := c.ShouldBindJSON(&req); err != nil || req.AgentID == "" || req.Model == "" || req.SystemPrompt == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "agent_id, model and system_prompt are required")
		return
	}
	a := &store.Assistant{
		AgentID:      req.AgentID,
		Name:         req.Name,
		Description:  req.Description,
		SystemPrompt: req.SystemPrompt,
		ModelID:      req.Model,
		Enabled:      true,
	}
	if req.Enabled != nil {
		a.Enabled = *req.Enabled
	}
	created, err := s.store.CreateAssistant(c.Request.Context(), a)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			abortWith(c, http.StatusConflict, "agent_exists", "an agent with this agent_id already exists")
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, assistantAdminJSON(created))
}

// handleUpdateAssistant partially updates a template (admin).
func (s *Server) handleUpdateAssistant(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req struct {
		Name         *string `json:"name"`
		Description  *string `json:"description"`
		SystemPrompt *string `json:"system_prompt"`
		Model        *string `json:"model"`
		Enabled      *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}
	updated, err := s.store.UpdateAssistant(c.Request.Context(), id, &store.AssistantPatch{
		Name:         req.Name,
		Description:  req.Description,
		SystemPrompt: req.SystemPrompt,
		ModelID:      req.Model,
		Enabled:      req.Enabled,
	})
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, assistantAdminJSON(updated))
}

// handleDeleteAssistant removes a template (admin).
func (s *Server) handleDeleteAssistant(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeleteAssistant(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
