package api

import (
	"sync"
	"time"
)

// P4-3: per-key in-memory rate limits on a fixed one-minute window.
// rpm/tpm <= 0 disables that limit. Single-binary approximation: with
// multiple server instances the window is per-instance (a shared store such
// as Redis would be needed for exact global limits).

type rateLimiter struct {
	mu  sync.Mutex
	rpm int
	tpm int
	win map[int64]*limiterWindow
}

type limiterWindow struct {
	minute int64 // Unix minute bucket
	reqs   int
	tokens int
}

func newRateLimiter(rpm, tpm int) *rateLimiter {
	return &rateLimiter{rpm: rpm, tpm: tpm, win: map[int64]*limiterWindow{}}
}

// Enabled reports whether any limit is configured.
func (rl *rateLimiter) Enabled() bool {
	return rl != nil && (rl.rpm > 0 || rl.tpm > 0)
}

// rateLimitMessage renders the 429 body for a blocked limit.
func rateLimitMessage(which string) string {
	if which == "tpm" {
		return "token rate limit exceeded (tokens/minute); retry in the next minute"
	}
	return "request rate limit exceeded (requests/minute); retry in the next minute"
}

// AllowRequest enforces the per-minute request limit and, when under it,
// counts the request. It also refuses when the key's token window is
// already exhausted. Returns (false, which limit) when blocked.
func (rl *rateLimiter) AllowRequest(keyID int64) (bool, string) {
	if rl == nil || !rl.Enabled() {
		return true, ""
	}
	return rl.allowRequestAt(keyID, time.Now())
}

func (rl *rateLimiter) allowRequestAt(keyID int64, now time.Time) (bool, string) {
	if rl == nil || !rl.Enabled() {
		return true, ""
	}
	minute := now.Unix() / 60
	rl.mu.Lock()
	defer rl.mu.Unlock()
	w := rl.windowLocked(keyID, minute)
	if rl.tpm > 0 && w.tokens >= rl.tpm {
		return false, "tpm"
	}
	if rl.rpm > 0 && w.reqs >= rl.rpm {
		return false, "rpm"
	}
	if rl.rpm > 0 {
		w.reqs++
	}
	return true, ""
}

// AddTokens records the actual tokens a finished request used, toward the
// key's token-per-minute window.
func (rl *rateLimiter) AddTokens(keyID int64, tokens int) {
	if rl == nil || rl.tpm <= 0 || tokens <= 0 {
		return
	}
	minute := time.Now().Unix() / 60
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.windowLocked(keyID, minute).tokens += tokens
}

func (rl *rateLimiter) windowLocked(keyID, minute int64) *limiterWindow {
	w := rl.win[keyID]
	if w == nil || w.minute != minute {
		w = &limiterWindow{minute: minute}
		rl.win[keyID] = w
	}
	// Lazy pruning keeps the map bounded (stale entries expire after a
	// couple of minutes; done opportunistically, no sweeper goroutine).
	if len(rl.win) > 1024 {
		for id, ww := range rl.win {
			if minute-ww.minute > 2 {
				delete(rl.win, id)
			}
		}
	}
	return w
}
