package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"modelhub/internal/billing"
	"modelhub/internal/store"
)

// --- admin: agent (reseller) management ---

func agentJSON(a *store.Agent, email string) gin.H {
	return gin.H{
		"id":         a.ID,
		"user_id":    a.UserID,
		"email":      email,
		"rate":       a.Rate,
		"created_at": a.CreatedAt,
	}
}

// handleListAgents lists all reseller agents with their user emails.
func (s *Server) handleListAgents(c *gin.Context) {
	rows, err := s.store.ListAgents(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(rows))
	for i := range rows {
		data = append(data, agentJSON(&rows[i].Agent, rows[i].Email))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

type createAgentReq struct {
	UserID int64   `json:"user_id"`
	Rate   float64 `json:"rate"`
}

// handleCreateAgent marks an existing billing user as a reseller.
// Rate is the wholesale multiplier in (0, 1]: 0.85 = pay 85% of retail.
func (s *Server) handleCreateAgent(c *gin.Context) {
	ctx := c.Request.Context()
	var req createAgentReq
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID == 0 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "user_id and rate are required")
		return
	}
	a, err := s.store.CreateAgent(ctx, req.UserID, req.Rate)
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if err != nil {
		if errors.Is(err, store.ErrInvalidRate) {
			abortWith(c, http.StatusBadRequest, "invalid_rate", "rate must be in (0, 1]")
			return
		}
		if strings.Contains(err.Error(), "duplicate key") {
			abortWith(c, http.StatusConflict, "already_agent", "user is already an agent")
			return
		}
		httpErr(c, err)
		return
	}
	email := ""
	if u, err := s.store.GetUser(ctx, a.UserID); err == nil {
		email = u.Email
	}
	c.JSON(http.StatusCreated, agentJSON(a, email))
}

type updateAgentReq struct {
	Rate float64 `json:"rate"`
}

// handleUpdateAgent changes an agent's wholesale rate.
func (s *Server) handleUpdateAgent(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req updateAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "rate is required")
		return
	}
	a, err := s.store.UpdateAgentRate(c.Request.Context(), id, req.Rate)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if errors.Is(err, store.ErrInvalidRate) {
		abortWith(c, http.StatusBadRequest, "invalid_rate", "rate must be in (0, 1]")
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	email := ""
	if u, err := s.store.GetUser(c.Request.Context(), a.UserID); err == nil {
		email = u.Email
	}
	c.JSON(http.StatusOK, agentJSON(a, email))
}

// handleDeleteAgent removes an agency; its subkeys become regular keys of
// the agent user.
func (s *Server) handleDeleteAgent(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeleteAgent(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// --- console: reseller subkeys (agent's own view) ---

// currentAgent resolves the caller's agent record; aborts with 403 when the
// caller is not a reseller.
func (s *Server) currentAgent(c *gin.Context) *store.Agent {
	u := userFrom(c)
	if u == nil {
		return nil
	}
	a, err := s.store.GetAgentByUserID(c.Request.Context(), u.ID)
	if err != nil {
		abortWith(c, http.StatusForbidden, "not_agent", "this account is not a reseller agent")
		return nil
	}
	return a
}

// handleListSubkeys lists the caller agent's reseller keys.
func (s *Server) handleListSubkeys(c *gin.Context) {
	a := s.currentAgent(c)
	if a == nil {
		return
	}
	keys, err := s.store.ListSubkeys(c.Request.Context(), a.ID)
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

type createSubkeyReq struct {
	Name          string   `json:"name"`
	Markup        *float64 `json:"markup"`
	QuotaUSD      *float64 `json:"quota_usd"`
	AllowedModels []string `json:"allowed_models"`
	ExpiresAt     *string  `json:"expires_at"`
}

// handleCreateSubkey mints a reseller key that bills the agent's balance at
// the agent's wholesale rate. The plain key is returned exactly once.
func (s *Server) handleCreateSubkey(c *gin.Context) {
	a := s.currentAgent(c)
	if a == nil {
		return
	}
	u := userFrom(c)
	var req createSubkeyReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	markup := 1.0
	if req.Markup != nil {
		if *req.Markup <= 0 || *req.Markup > 100 {
			abortWith(c, http.StatusBadRequest, "invalid_request", "markup must be in (0, 100]")
			return
		}
		markup = *req.Markup
	}
	var quota *int64
	if req.QuotaUSD != nil {
		if *req.QuotaUSD < 0 {
			abortWith(c, http.StatusBadRequest, "invalid_request", "quota_usd must be >= 0")
			return
		}
		q := billing.ToMicro(*req.QuotaUSD)
		quota = &q
	}
	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			abortWith(c, http.StatusBadRequest, "invalid_request", "expires_at must be RFC3339 (e.g. 2026-01-01T00:00:00Z)")
			return
		}
		expiresAt = &t
	}
	plain, key, err := s.store.CreateSubkey(c.Request.Context(), a.ID, u.ID, req.Name, markup, req.AllowedModels, quota, expiresAt)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"key": plain, "id": key.ID, "name": key.Name})
}

// handleDeleteSubkey removes one of the caller agent's reseller keys.
func (s *Server) handleDeleteSubkey(c *gin.Context) {
	a := s.currentAgent(c)
	if a == nil {
		return
	}
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeleteSubkey(c.Request.Context(), a.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
