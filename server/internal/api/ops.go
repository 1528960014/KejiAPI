package api

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

// Version is the build version, overridable at build time with
// -ldflags "-X kejiapi/internal/api.Version=1.2.3".
var Version = "1.0.0"

var bootTime = time.Now()

// handleOps returns read-only runtime diagnostics for the admin
// "ops monitoring" panel: process uptime / memory / goroutines,
// database pool health, and configured rate limits.
func (s *Server) handleOps(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	db := gin.H{"status": "ok", "latency_ms": 0}
	if pool := s.store.Pool(); pool != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		t0 := time.Now()
		err := pool.Ping(ctx)
		latency := int(time.Since(t0).Milliseconds())
		cancel()
		if err != nil {
			db["status"] = "down"
		} else {
			db["latency_ms"] = latency
		}
		st := pool.Stat()
		db["total_conns"] = st.TotalConns()
		db["idle_conns"] = st.IdleConns()
		db["acquired_conns"] = st.AcquiredConns()
		db["max_conns"] = st.MaxConns()
	} else {
		db["status"] = "down"
	}

	c.JSON(http.StatusOK, gin.H{
		"version":    Version,
		"time":       time.Now().UTC().Format(time.RFC3339),
		"uptime":     int64(time.Since(bootTime).Seconds()),
		"goroutines": runtime.NumGoroutine(),
		"mem": gin.H{
			"alloc_mb": m.Alloc / 1024 / 1024,
			"sys_mb":   m.Sys / 1024 / 1024,
			"num_gc":   m.NumGC,
		},
		"database": db,
		"limits": gin.H{
			"rpm":              s.cfg.RPM,
			"tpm":              s.cfg.TPM,
			"conc_per_key":     s.cfg.ConcPerKey,
			"conc_per_channel": s.cfg.ConcPerChannel,
		},
	})
}
