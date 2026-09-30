package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
//   - Cooldown state is the shared gateway.ChannelHealth (per-instance
//     in-memory, also consulted by the task workers, P6-1).
//   - Non-retryable upstream errors (400/404/...) are forwarded as-is; the
//     request would fail identically on every channel.

const dialErrBodyLimit = 64 * 1024

// errNoUsableChannel: the model has no enabled channel, or every enabled
// channel is inside its cooldown window.
var errNoUsableChannel = errors.New("no usable channel")

// errConcurrencyLimited (P7-3): every usable channel is at its in-flight
// concurrency limit. Callers should surface this as 429 concurrency_limited.
var errConcurrencyLimited = errors.New("channel concurrency limit reached")

// channelSource is the subset of the store the dial loop needs, so the
// failover logic is unit-testable without a database.
type channelSource interface {
	ChannelsForModel(ctx context.Context, modelID string) ([]*store.Channel, error)
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
		if !s.health.IsDown(ch.ID, now) {
			return true, nil
		}
	}
	return false, nil
}

// dialChat POSTs the prepared chat body to the model's channels in priority
// order until one succeeds or a non-retryable error is reached.
//
// Returns:
//   - (attempt with res, nil)          — usable 2xx response (the caller
//     owns the in-flight concurrency slot and must release it)
//   - (attempt with status>=400, nil)  — final upstream error to forward
//   - (nil, errNoUsableChannel)        — no enabled/usable channel
//   - (nil, errConcurrencyLimited)     — P7-3: every usable channel is at
//     its in-flight limit
//   - (nil, err)                       — every channel had a transport failure
func (s *Server) dialChat(ctx context.Context, modelID string, body []byte, keyID int64) (*upstreamAttempt, error) {
	channels, err := s.channelSource.ChannelsForModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var last *upstreamAttempt
	var lastErr error
	var skippedLimit bool
	for _, ch := range channels {
		if s.health.IsDown(ch.ID, now) {
			continue
		}
		// P7-3: take an in-flight slot before dialing; a channel at its
		// limit is skipped without marking it failed.
		if !s.conc.Begin(keyID, ch.ID) {
			skippedLimit = true
			continue
		}
		res, dErr := s.provider.Chat(ctx, ch, body)
		if dErr != nil {
			s.conc.End(keyID, ch.ID)
			s.health.MarkFailed(ch.ID, now)
			lastErr = fmt.Errorf("channel %s: %w", ch.Name, dErr)
			slog.Warn("channel transport failure, failing over",
				"channel", ch.Name, "model", modelID, "error", dErr)
			continue
		}
		if res.StatusCode < 400 {
			s.health.MarkOK(ch.ID)
			// Slot stays held: the caller owns the live response and must
			// call s.conc.End(keyID, ch.ID) when the response finishes.
			return &upstreamAttempt{ch: ch, res: res}, nil
		}
		s.conc.End(keyID, ch.ID)
		errBody, _ := io.ReadAll(io.LimitReader(res.Body, dialErrBodyLimit))
		_ = res.Body.Close()
		if !gateway.UpstreamRetryable(res.StatusCode, false) {
			return &upstreamAttempt{ch: ch, status: res.StatusCode, errBody: errBody, header: res.Header}, nil
		}
		s.health.MarkFailed(ch.ID, now)
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
	if skippedLimit {
		return nil, errConcurrencyLimited
	}
	return nil, errNoUsableChannel
}

// dialJSON is the DoJSON variant of the failover loop (audio/speech and
// other single-shot upstream calls).
func (s *Server) dialJSON(ctx context.Context, modelID, method, path string, body []byte, extraHeaders map[string]string, keyID int64) (int, []byte, error) {
	channels, err := s.channelSource.ChannelsForModel(ctx, modelID)
	if err != nil {
		return 0, nil, err
	}
	now := time.Now()
	var lastStatus int
	var lastBody []byte
	var lastErr error
	var skippedLimit bool
	for _, ch := range channels {
		if s.health.IsDown(ch.ID, now) {
			continue
		}
		// P7-3: same in-flight slot management as dialChat.
		if !s.conc.Begin(keyID, ch.ID) {
			skippedLimit = true
			continue
		}
		status, respBody, dErr := s.provider.DoJSON(ctx, ch, method, path, body, extraHeaders)
		if dErr != nil {
			s.conc.End(keyID, ch.ID)
			s.health.MarkFailed(ch.ID, now)
			lastErr = fmt.Errorf("channel %s: %w", ch.Name, dErr)
			slog.Warn("channel transport failure, failing over",
				"channel", ch.Name, "model", modelID, "error", dErr)
			continue
		}
		s.conc.End(keyID, ch.ID)
		if status < 400 {
			s.health.MarkOK(ch.ID)
			return status, respBody, nil
		}
		if !gateway.UpstreamRetryable(status, false) {
			return status, respBody, nil
		}
		s.health.MarkFailed(ch.ID, now)
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
	if skippedLimit {
		return 0, nil, errConcurrencyLimited
	}
	return 0, nil, errNoUsableChannel
}
