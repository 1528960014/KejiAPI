package gateway

import (
	"net/http"
	"sync"
	"time"
)

// P5-1/P6-1: shared channel failover primitives. The API dial loop (chat /
// audio) and the task workers (media / drama) share the same retryable-error
// classification and the same in-memory channel health, so a channel that
// fails an HTTP request also cools down before the workers retry it.

// ChannelCooldownDuration is how long a failed channel is skipped.
const ChannelCooldownDuration = 5 * time.Minute

// UpstreamRetryable reports whether a failed upstream attempt might succeed
// on a different channel. Transport failures always retry; 401/403 mean the
// upstream rejected this channel's key; 408/429/5xx are transient. 400/404
// and friends are request problems — identical on every channel.
func UpstreamRetryable(status int, transportErr bool) bool {
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

// ChannelHealth tracks recent channel failures for the cooldown window.
// Per-instance in-memory state (documented trade-off: multi-instance
// deployments each track their own channel health).
type ChannelHealth struct {
	mu   sync.Mutex
	down map[int64]time.Time // channelID -> last failure time
}

func NewChannelHealth() *ChannelHealth {
	return &ChannelHealth{down: map[int64]time.Time{}}
}

// MarkFailed records a failure (starts/extends the cooldown).
func (h *ChannelHealth) MarkFailed(id int64, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.down[id] = now
	// Lazy prune (same pattern as the rate limiter), no sweeper goroutine.
	if len(h.down) > 1024 {
		for cid, t := range h.down {
			if now.Sub(t) > ChannelCooldownDuration*2 {
				delete(h.down, cid)
			}
		}
	}
}

// MarkOK clears the failure record after a success.
func (h *ChannelHealth) MarkOK(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.down, id)
}

// IsDown reports whether the channel is inside its cooldown window.
func (h *ChannelHealth) IsDown(id int64, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.down[id]
	return ok && now.Sub(t) < ChannelCooldownDuration
}

// CooldownUntil returns when a down channel becomes eligible again.
func (h *ChannelHealth) CooldownUntil(id int64, now time.Time) (time.Time, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.down[id]
	if !ok || now.Sub(t) >= ChannelCooldownDuration {
		return time.Time{}, false
	}
	return t.Add(ChannelCooldownDuration), true
}
