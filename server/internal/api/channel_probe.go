package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"kejiapi/internal/store"
)

// probeChannelOnce runs a minimal non-streaming completion (max_tokens=1)
// through one channel and returns the probe result. The same shape is used
// by the on-demand "test" endpoint, the probe-all endpoint and the
// background probe loop.
func (s *Server) probeChannelOnce(ctx context.Context, ch *store.Channel) store.ChannelHealthRow {
	h := store.ChannelHealthRow{ChannelID: ch.ID}
	m, err := s.store.GetModel(ctx, ch.ModelID)
	if err != nil {
		h.Error = "model " + ch.ModelID + " not configured"
		return h
	}
	body, _ := json.Marshal(map[string]any{
		"model":      m.UpstreamModel,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
		"stream":     false,
	})
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	start := time.Now()
	status, raw, derr := s.provider.DoJSON(tctx, ch, http.MethodPost, "/chat/completions", body, nil)
	h.LatencyMs = time.Since(start).Milliseconds()
	h.Status = status
	if derr != nil {
		h.Error = derr.Error()
		return h
	}
	if status >= 300 {
		msg := string(raw)
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		h.Error = msg
		return h
	}
	h.OK = true
	return h
}

// probeChannels probes a set of channels with bounded parallelism (8 at a
// time) and persists each result to channel_health.
func (s *Server) probeChannels(ctx context.Context, channels []store.Channel) map[int64]store.ChannelHealthRow {
	out := map[int64]store.ChannelHealthRow{}
	if len(channels) == 0 {
		return out
	}
	sem := make(chan struct{}, 8)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range channels {
		ch := channels[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			h := s.probeChannelOnce(pctx, &ch)
			if err := s.store.UpsertChannelHealth(pctx, &h); err != nil {
				slog.Warn("probe: save snapshot", "channel_id", ch.ID, "error", err)
			}
			mu.Lock()
			out[ch.ID] = h
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// handleProbeAllChannels probes every channel on demand ("probe all
// routes") so the admin can check all line healths at once.
func (s *Server) handleProbeAllChannels(c *gin.Context) {
	ctx := c.Request.Context()
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		httpErr(c, err)
		return
	}
	results := s.probeChannels(ctx, channels)
	okCount := 0
	for _, h := range results {
		if h.OK {
			okCount++
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": results, "count": len(results), "ok": okCount})
}

// StartProbeLoop periodically probes all enabled channels so the admin
// route table always shows a fresh line status. It runs one pass 30s after
// boot, then every 10 minutes.
func (s *Server) StartProbeLoop(ctx context.Context) {
	const interval = 10 * time.Minute
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}
	s.runProbePass(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runProbePass(ctx)
		}
	}
}

func (s *Server) runProbePass(ctx context.Context) {
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		slog.Warn("probe: list channels", "error", err)
		return
	}
	on := make([]store.Channel, 0, len(channels))
	for _, ch := range channels {
		if ch.Enabled {
			on = append(on, ch)
		}
	}
	if len(on) == 0 {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	results := s.probeChannels(pctx, on)
	for id, h := range results {
		if !h.OK {
			slog.Warn("probe: channel unhealthy", "channel_id", id, "status", h.Status, "error", h.Error)
		}
	}
}
