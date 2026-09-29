package gateway

import (
	"testing"
	"time"
)

func TestUpstreamRetryable(t *testing.T) {
	cases := []struct {
		status int
		trans  bool
		want   bool
	}{
		{0, true, true},    // transport failure
		{401, false, true}, // channel key rejected upstream
		{403, false, true},
		{408, false, true},
		{429, false, true},
		{500, false, true},
		{502, false, true},
		{503, false, true},
		{504, false, true},
		{200, false, false},
		{400, false, false}, // request problem: identical on every channel
		{404, false, false},
		{422, false, false},
	}
	for _, tc := range cases {
		if got := UpstreamRetryable(tc.status, tc.trans); got != tc.want {
			t.Errorf("UpstreamRetryable(%d, %v) = %v, want %v", tc.status, tc.trans, got, tc.want)
		}
	}
}

func TestChannelHealthCooldown(t *testing.T) {
	h := NewChannelHealth()
	t0 := time.Now()
	if h.IsDown(1, t0) {
		t.Fatal("fresh channel must not be down")
	}
	h.MarkFailed(1, t0)
	if !h.IsDown(1, t0.Add(time.Minute)) {
		t.Fatal("channel should be down right after a failure")
	}
	if h.IsDown(1, t0.Add(ChannelCooldownDuration+time.Second)) {
		t.Fatal("channel should be back after the cooldown window")
	}
	h.MarkFailed(2, t0)
	h.MarkOK(2)
	if h.IsDown(2, t0.Add(time.Minute)) {
		t.Fatal("MarkOK must clear the failure")
	}
	if _, down := h.CooldownUntil(1, t0.Add(time.Minute)); !down {
		t.Fatal("CooldownUntil should report a down channel")
	}
	if until, down := h.CooldownUntil(1, t0.Add(time.Minute)); !down ||
		until != t0.Add(ChannelCooldownDuration) {
		t.Fatalf("unexpected cooldown until: %v %v", until, down)
	}
}
