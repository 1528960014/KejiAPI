package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"kejiapi/internal/store"
)

// P7-6: external systems. Admin-managed third-party pages rendered as
// iframes by the admin console. URL validation is intentionally loose
// (http/https only): the iframe inherits the browser's own origin, CSP and
// X-Frame-Options enforcement for the embedded target.

func validExternalURL(url string) bool {
	u := strings.TrimSpace(url)
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// handleListExternalPages lists all registered pages (admin).
func (s *Server) handleListExternalPages(c *gin.Context) {
	list, err := s.store.ListExternalPages(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

type externalPageReq struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	SortOrder *int   `json:"sort_order"`
}

// handleCreateExternalPage registers a page (admin).
func (s *Server) handleCreateExternalPage(c *gin.Context) {
	var req externalPageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if len(req.Name) < 2 || len(req.Name) > 64 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "name must be 2-64 characters")
		return
	}
	if !validExternalURL(req.URL) {
		abortWith(c, http.StatusBadRequest, "invalid_request", "url must start with http:// or https://")
		return
	}
	sort := 0
	if req.SortOrder != nil {
		sort = *req.SortOrder
	}
	created, err := s.store.CreateExternalPage(c.Request.Context(), req.Name, req.URL, sort)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

// handleUpdateExternalPage partially updates a page (admin).
func (s *Server) handleUpdateExternalPage(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req struct {
		Name      *string `json:"name"`
		URL       *string `json:"url"`
		Enabled   *bool   `json:"enabled"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if len(trimmed) < 2 || len(trimmed) > 64 {
			abortWith(c, http.StatusBadRequest, "invalid_request", "name must be 2-64 characters")
			return
		}
		req.Name = &trimmed
	}
	if req.URL != nil {
		trimmed := strings.TrimSpace(*req.URL)
		if !validExternalURL(trimmed) {
			abortWith(c, http.StatusBadRequest, "invalid_request", "url must start with http:// or https://")
			return
		}
		req.URL = &trimmed
	}
	updated, err := s.store.UpdateExternalPage(c.Request.Context(), id, &store.ExternalPagePatch{
		Name:      req.Name,
		URL:       req.URL,
		Enabled:   req.Enabled,
		SortOrder: req.SortOrder,
	})
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

// handleDeleteExternalPage removes a page (admin).
func (s *Server) handleDeleteExternalPage(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeleteExternalPage(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
