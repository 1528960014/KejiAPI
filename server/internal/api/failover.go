package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"modelhub/internal/gateway"
	"modelhub/internal/store"
)

// P5-1: automatic channel failover with a short failure cooldown. When a
// channel errors (transport failure or a retryable HTTP status), the gateway
// dials the next-priority channel for the same model. A failed channel enters
// a cooldown so a dead channel is not retried on every single request.
//
// Boundaries (documented):
//   - Failover only happens BEFORE the response starts. A stream that breaks
//     mid-flight is not retried (the client already has partial output).
//   - Cooldown state is per-instance in-memory (same trade-off as the P4-3
//     limiter): each gateway instance tracks its own channel health.
//   - Non-retryable upstream errors (400/404/...) are forwarded as-is; the
//     request would fail identically on every channel.

const (
	channelCooldownDuration = 5 * time.Minute
	dialErrBodyLimit        = 64 * 1024
)

// errNoUsableChannel: the model has no enabled channel, or every enabled
// channel is inside its cooldown window.
var errNoUsableChannel = errors.New("no usable channel")

// channelSource is the subset of the store the dial loop needs, so the
// failover logic is unit-testable without a database.
type channelSource interface {
	ChannelsForModel(ctx context.Context, modelID string) ([]*store.Channel, error)
}

// upstreamRetryable reports whether a failed upstream attempt might succeed
// on a different channel. Transport failures always retry; 401/403 mean the
// upstream rejected this channel's key; 408/429/5xx are transient. 400/404
// and friends are request problems — identical on every channel.
func upstreamRetryable(status int, transportErr bool) bool {
	if transportErr {
		return true
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestTimeout,
		http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// channelHealth tracks recent channel failures for the cooldown window.
type channelHealth struct {
	mu   sync.Mutex
	down map[int64]time.Time // channelID -> last failure time
}

func newChannelHealth() *channelHealth {
	return &channelHealth{down: map[int64]time.Time{}}
}

func (h *channelHealth) markFailed(id int64, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.down[id] = now
	// Lazy prune (same pattern as the rate limiter), no sweeper goroutine.
	if len(h.down) > 1024 {
		for cid, t := range h.down {
			if now.Sub(t) > channelCooldownDuration*2 {
				delete(h.down, cid)
			}
		}
	}
}

func (h *channelHealth) markOK(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.down, id)
}

// isDown reports whether the channel is inside its cooldown window.
func (h *channelHealth) isDown(id int64, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.down[id]
	return ok && now.Sub(t) < channelCooldownDuration
}

// cooldownUntil returns when a down channel becomes eligible again.
func (h *channelHealth) cooldownUntil(id int64, now time.Time) (time.Time, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.down[id]
	if !ok || now.Sub(t) >= channelCooldownDuration {
		return time.Time{}, false
	}
	return t.Add(channelCooldownDuration), true
}

// upstreamAttempt is the outcome of the dialChat loop.
type upstreamAttempt struct {
	ch      *store.Channel
	res     *gateway.ChatResult // live response on success; caller must Close
	status  int                 // >=400 when res == nil
	errBody []byte
	header  http.Header
}

// hasUsableChannel reports whether at least one enabled, non-cooled-down
// channel exists for the model — a fast pre-hold check that preserves the
// no_channel 502 before any funds are held.
func (s *Server) hasUsableChannel(ctx context.Context, modelID string) (bool, error) {
	chs, err := s.channelSource.ChannelsForModel(ctx, modelID)
	if err != nil {
		return false, err
	}
	now := time.Now()
	for _, ch := range chs {
		if !s.health.isDown(ch.ID, now) {
			return true, nil
		}
	}
	return false, nil
}

// dialChat POSTs the prepared chat body to the model's channels in priority
// order until one succeeds or a non-retryable error is reached.
//
// Returns:
//   - (attempt with res, nil)        — usable 2xx response
//   - (attempt with status>=400, nil) — final upstream error to forward
//   - (nil, errNoUsableChannel)      — no enabled/usable channel
//   - (nil, err)                     — every channel had a transport failure
func (s *Server) dialChat(ctx context.Context, modelID string, body []byte) (*upstreamAttempt, error) {
	channels, err := s.channelSource.ChannelsForModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var last *upstreamAttempt
	var lastErr error
	for _, ch := range channels {
		if s.health.isDown(ch.ID, now) {
			continue
		}
		res, dErr := s.provider.Chat(ctx, ch, body)
		if dErr != nil {
			s.health.markFailed(ch.ID, now)
			lastErr = fmt.Errorf("channel %s: %w", ch.Name, dErr)
			slog.Warn("channel transport failure, failing over",
				"channel", ch.Name, "model", modelID, "error", dErr)
			continue
		}
		if res.StatusCode < 400 {
			s.health.markOK(ch.ID)
			return &upstreamAttempt{ch: ch, res: res}, nil
		}
		errBody, _ := io.ReadAll(io.LimitReader(res.Body, dialErrBodyLimit))
		_ = res.Body.Close()
		if !upstreamRetryable(res.StatusCode, false) {
			return &upstreamAttempt{ch: ch, status: res.StatusCode, errBody: errBody, header: res.Header}, nil
		}
		s.health.markFailed(ch.ID, now)
		last = &upstreamAttempt{ch: ch, status: res.StatusCode, errBody: errBody, header: res.Header}
		slog.Warn("channel returned retryable error, failing over",
			"channel", ch.Name, "model", modelID, "status", res.StatusCode)
	}
	if last != nil {
		return last, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errNoUsableChannel
}

// dialJSON is the DoJSON variant of the failover loop (audio/speech and
// other single-shot upstream calls).
func (s *Server) dialJSON(ctx context.Context, modelID, method, path string, body []byte, extraHeaders map[string]string) (int, []byte, error) {
	channels, err := s.channelSource.ChannelsForModel(ctx, modelID)
	if err != nil {
		return 0, nil, err
	}
	now := time.Now()
	var lastStatus int
	var lastBody []byte
	var lastErr error
	for _, ch := range channels {
		if s.health.isDown(ch.ID, now) {
			continue
		}
		status, respBody, dErr := s.provider.DoJSON(ctx, ch, method, path, body, extraHeaders)
		if dErr != nil {
			s.health.markFailed(ch.ID, now)
			lastErr = fmt.Errorf("channel %s: %w", ch.Name, dErr)
			slog.Warn("channel transport failure, failing over",
				"channel", ch.Name, "model", modelID, "error", dErr)
			continue
		}
		if status < 400 {
			s.health.markOK(ch.ID)
			return status, respBody, nil
		}
		if !upstreamRetryable(status, false) {
			return status, respBody, nil
		}
		s.health.markFailed(ch.ID, now)
		lastStatus, lastBody = status, respBody
		slog.Warn("channel returned retryable error, failing over",
			"channel", ch.Name, "model", modelID, "status", status)
	}
	if lastStatus != 0 {
		return lastStatus, lastBody, nil
	}
	if lastErr != nil {
		return 0, nil, lastErr
	}
	return 0, nil, errNoUsableChannel
}