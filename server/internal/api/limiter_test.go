package api

import (
	"testing"
	"time"
)

func TestRateLimiterRPM(t *testing.T) {
	rl := newRateLimiter(2, 0)
	t0 := time.Date(2026, 1, 1, 12, 0, 30, 0, time.UTC)

	ok, _ := rl.allowRequestAt(1, t0)
	if !ok {
		t.Fatal("first request must pass")
	}
	ok, _ = rl.allowRequestAt(1, t0)
	if !ok {
		t.Fatal("second request must pass")
	}
	if ok, which := rl.allowRequestAt(1, t0); ok || which != "rpm" {
		t.Fatalf("third request = %v, %q; want blocked by rpm", ok, which)
	}
	// another key has its own window
	if ok, _ := rl.allowRequestAt(2, t0); !ok {
		t.Fatal("other key must not be blocked")
	}
	// the next minute starts fresh
	if ok, _ := rl.allowRequestAt(1, t0.Add(time.Minute)); !ok {
		t.Fatal("new minute must reset the window")
	}
}

func TestRateLimiterTPM(t *testing.T) {
	rl := newRateLimiter(0, 100)
	t0 := time.Date(2026, 1, 1, 12, 0, 30, 0, time.UTC)

	if ok, _ := rl.allowRequestAt(1, t0); !ok {
		t.Fatal("request before tokens must pass")
	}
	rl.windowLocked(1, t0.Unix()/60).tokens += 100
	if ok, which := rl.allowRequestAt(1, t0); ok || which != "tpm" {
		t.Fatalf("request over TPM = %v, %q; want blocked by tpm", ok, which)
	}
	if ok, _ := rl.allowRequestAt(1, t0.Add(time.Minute)); !ok {
		t.Fatal("new minute must reset the token window")
	}
}

func TestRateLimiterAddTokens(t *testing.T) {
	rl := newRateLimiter(100, 50)
	t0 := time.Now()
	rl.AddTokens(7, 30)
	rl.AddTokens(7, 25) // 55 > 50
	rl.mu.Lock()
	tokens := rl.win[7].tokens
	rl.mu.Unlock()
	if tokens != 55 {
		t.Fatalf("tokens = %d, want 55", tokens)
	}
	if ok, which := rl.allowRequestAt(7, t0); ok || which != "tpm" {
		t.Fatalf("over TPM = %v, %q; want blocked", ok, which)
	}
}

func TestRateLimiterDisabled(t *testing.T) {
	rl := newRateLimiter(0, 0)
	if rl.Enabled() {
		t.Fatal("0/0 must be disabled")
	}
	if ok, _ := rl.allowRequestAt(1, time.Now()); !ok {
		t.Fatal("disabled limiter must never block")
	}
	var nilRL *rateLimiter
	if ok, _ := nilRL.allowRequestAt(1, time.Now()); !ok {
		t.Fatal("nil limiter must never block")
	}
}
