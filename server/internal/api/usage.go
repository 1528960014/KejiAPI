package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"kejiapi/internal/billing"
	"kejiapi/internal/store"
)

// handleListUsage returns recent usage records, optionally for one key.
func (s *Server) handleListUsage(c *gin.Context) {
	filter := store.UsageFilter{Limit: 50, Offset: 0}
	if idStr := c.Query("api_key_id"); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			abortWith(c, http.StatusBadRequest, "invalid_id", "api_key_id must be an integer")
			return
		}
		filter.APIKeyID = &id
	}
	filter.Limit, filter.Offset = parsePagination(c, 50, 500)
	logs, err := s.store.ListUsage(c.Request.Context(), filter)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(logs))
	for _, l := range logs {
		item := gin.H{
			"id":                l.ID,
			"model":             l.ModelID,
			"provider":          l.Provider,
			"stream":            l.Stream,
			"prompt_tokens":     l.PromptTokens,
			"completion_tokens": l.CompletionTokens,
			"cost_usd":          l.Cost,
			"status":            l.Status,
			"error":             l.ErrorMsg,
			"created_at":        l.CreatedAt,
		}
		if l.APIKeyID != nil {
			item["api_key_id"] = *l.APIKeyID
		}
		data = append(data, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleUsageSummary aggregates usage, optionally for one key and/or since a
// timestamp (RFC3339).
func (s *Server) handleUsageSummary(c *gin.Context) {
	var keyID *int64
	if idStr := c.Query("api_key_id"); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			abortWith(c, http.StatusBadRequest, "invalid_id", "api_key_id must be an integer")
			return
		}
		keyID = &id
	}
	var since *time.Time
	if ts := c.Query("since"); ts != "" {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			abortWith(c, http.StatusBadRequest, "invalid_since", "since must be RFC3339, e.g. 2026-09-01T00:00:00Z")
			return
		}
		since = &t
	}
	summary, err := s.store.UsageSummary(c.Request.Context(), keyID, since)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"requests":          summary.Requests,
		"prompt_tokens":     summary.PromptTokens,
		"completion_tokens": summary.CompletionTokens,
		"cost_micro":        summary.CostMicro,
		"cost_usd":          billing.USD(summary.CostMicro),
	})
}
