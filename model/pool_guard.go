package model

import (
	"sync"
	"time"

	"kejiapi/setting/operation_setting"
)

// Per-account sliding-window pacing (ban prevention). The window stores
// unix seconds of requests actually served by each pooled channel.
var (
	poolGuardMu      sync.Mutex
	poolGuardWindows = make(map[int][]int64)
)

func poolGuardWindowSeconds() int64 {
	setting := operation_setting.GetAccountPoolSetting()
	if setting.RateLimitWindowMinutes > 0 {
		return int64(setting.RateLimitWindowMinutes) * 60
	}
	return 300 * 60
}

func poolGuardLimit() int {
	setting := operation_setting.GetAccountPoolSetting()
	if setting.RateLimitEnabled && setting.RateLimitRequests > 0 {
		return setting.RateLimitRequests
	}
	return 0
}

// PoolGuardAllows reports whether the channel may serve another request
// under the account pool pacing policy. Channels that are not part of the
// pool always pass.
func PoolGuardAllows(channelId int) bool {
	limit := poolGuardLimit()
	if limit <= 0 {
		return true
	}
	now := time.Now().Unix()
	min := now - poolGuardWindowSeconds()

	poolGuardMu.Lock()
	defer poolGuardMu.Unlock()
	stamps := poolGuardWindows[channelId]
	if len(stamps) == 0 {
		return true
	}
	keep := 0
	for _, ts := range stamps {
		if ts >= min {
			stamps[keep] = ts
			keep++
		}
	}
	poolGuardWindows[channelId] = stamps[:keep]
	return len(poolGuardWindows[channelId]) < limit
}

// PoolGuardRecord counts one request actually served by the channel.
func PoolGuardRecord(channelId int) {
	if poolGuardLimit() <= 0 {
		return
	}
	now := time.Now().Unix()
	min := now - poolGuardWindowSeconds()

	poolGuardMu.Lock()
	defer poolGuardMu.Unlock()
	stamps := poolGuardWindows[channelId]
	keep := 0
	for _, ts := range stamps {
		if ts >= min {
			stamps[keep] = ts
			keep++
		}
	}
	poolGuardWindows[channelId] = append(stamps[:keep], now)
}
