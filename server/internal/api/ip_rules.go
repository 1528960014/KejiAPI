package api

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"kejiapi/internal/store"
)

// IP gateway access control. Rules are cached in memory (s.ipRules) and
// reloaded on every CRUD change; ipGate runs before API-key auth so blocked
// IPs never reach key validation.

func (s *Server) reloadIPRules(ctx context.Context) error {
	rules, err := s.store.ListIPRules(ctx)
	if err != nil {
		return err
	}
	s.ipMu.Lock()
	s.ipRules = rules
	s.ipMu.Unlock()
	return nil
}

// ipGate enforces blacklist/whitelist rules on the client IP. If any
// enabled whitelist rule exists, only matching IPs are allowed.
func (s *Server) ipGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		ipStr := c.ClientIP()
		if ipStr == "" {
			c.Next()
			return
		}
		s.ipMu.RLock()
		rules := s.ipRules
		s.ipMu.RUnlock()
		if len(rules) == 0 {
			c.Next()
			return
		}
		whitelistOnly := false
		for _, r := range rules {
			if r.Enabled && r.Kind == "whitelist" {
				whitelistOnly = true
				break
			}
		}
		for _, r := range rules {
			if !r.Enabled {
				continue
			}
			if ip := net.ParseIP(ipStr); ip != nil && store.IPRuleMatches(r.CIDR, ip) {
				if r.Kind == "blacklist" {
					abortWith(c, http.StatusForbidden, "ip_blocked", "IP address is not allowed")
					return
				}
				c.Next()
				return
			}
		}
		if whitelistOnly {
			abortWith(c, http.StatusForbidden, "ip_blocked", "IP address is not allowed")
			return
		}
		c.Next()
	}
}

func ipRuleJSON(r *store.IPRule) gin.H {
	return gin.H{
		"id":         r.ID,
		"kind":       r.Kind,
		"cidr":       r.CIDR,
		"note":       r.Note,
		"enabled":    r.Enabled,
		"created_at": r.CreatedAt,
	}
}

func (s *Server) handleListIPRules(c *gin.Context) {
	rules, err := s.store.ListIPRules(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(rules))
	for i := range rules {
		data = append(data, ipRuleJSON(&rules[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func (s *Server) handleCreateIPRule(c *gin.Context) {
	var body struct {
		Kind string `json:"kind"`
		CIDR string `json:"cidr"`
		Note string `json:"note"`
	}
	if err := c.BindJSON(&body); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	r, err := s.store.CreateIPRule(c.Request.Context(), body.Kind, body.CIDR, body.Note)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := s.reloadIPRules(c.Request.Context()); err != nil {
		c.JSON(http.StatusCreated, ipRuleJSON(r))
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, ipRuleJSON(r))
}

func (s *Server) handleUpdateIPRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "invalid id")
		return
	}
	var body struct {
		Note    *string `json:"note"`
		Enabled *bool   `json:"enabled"`
	}
	if err := c.BindJSON(&body); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	// Merge over current values so partial PATCH works.
	current := map[int64]store.IPRule{}
	if rules, err := s.store.ListIPRules(c.Request.Context()); err == nil {
		for _, r := range rules {
			current[r.ID] = r
		}
	}
	prev, ok := current[id]
	if !ok {
		abortWith(c, http.StatusNotFound, "not_found", "ip rule not found")
		return
	}
	note := prev.Note
	if body.Note != nil {
		note = *body.Note
	}
	enabled := prev.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	r, err := s.store.UpdateIPRule(c.Request.Context(), id, note, enabled)
	if err != nil {
		httpErr(c, err)
		return
	}
	if err := s.reloadIPRules(c.Request.Context()); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, ipRuleJSON(r))
}

func (s *Server) handleDeleteIPRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "invalid id")
		return
	}
	if err := s.store.DeleteIPRule(c.Request.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") || err == store.ErrNotFound {
			abortWith(c, http.StatusNotFound, "not_found", "ip rule not found")
			return
		}
		httpErr(c, err)
		return
	}
	if err := s.reloadIPRules(c.Request.Context()); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
