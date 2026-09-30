package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"kejiapi/internal/store"
)

// auditLog records every mutating admin request (POST/PATCH/PUT/DELETE)
// after it completes. Runs after authMasterKey so only authenticated
// admin actions are logged. The write is async and best-effort.
func (s *Server) auditLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		method := c.Request.Method
		if method != "POST" && method != "PATCH" && method != "PUT" && method != "DELETE" {
			return
		}
		target := make([]string, 0, len(c.Params))
		for _, p := range c.Params {
			target = append(target, p.Key+"="+p.Value)
		}
		entry := store.AuditLog{
			Action: method + " " + c.FullPath(),
			Target: strings.Join(target, " "),
			Status: c.Writer.Status(),
			IP:     c.ClientIP(),
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.store.AddAuditLog(ctx, entry); err != nil {
				// best-effort: audit failures must not affect the request
			}
		}()
	}
}

func auditLogJSON(l store.AuditLog) gin.H {
	return gin.H{
		"id":         l.ID,
		"action":     l.Action,
		"target":     l.Target,
		"status":     l.Status,
		"ip":         l.IP,
		"created_at": l.CreatedAt,
	}
}

func (s *Server) handleListAuditLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(c.Query("offset"))
	if offset < 0 {
		offset = 0
	}
	logs, total, err := s.store.ListAuditLogs(c.Request.Context(), limit, offset)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(logs))
	for i := range logs {
		data = append(data, auditLogJSON(logs[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "total": total})
}
