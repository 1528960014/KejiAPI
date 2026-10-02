package operation_setting

import "kejiapi/setting/config"

// AccountPoolSetting controls the account pool ban-prevention policy:
// timed cooldown when upstream quota errors occur, isolation tagging for
// banned accounts, per-account request pacing, and session stickiness
// (keeping each API key on the same pooled account per model and group).
type AccountPoolSetting struct {
	// CooldownEnabled automatically disables (with a timed marker)
	// subscription channels that hit upstream quota / rate-limit errors,
	// and re-enables them after CooldownMinutes.
	CooldownEnabled bool `json:"cooldown_enabled"`
	// CooldownMinutes is the default cooldown duration in minutes.
	CooldownMinutes int `json:"cooldown_minutes"`
	// BanIsolateEnabled tags credential / ban errors so the UI can show
	// the affected accounts as isolated. Isolated accounts are NOT
	// auto-restored.
	BanIsolateEnabled bool `json:"ban_isolate_enabled"`
	// RateLimitEnabled paces outgoing requests per pooled account.
	RateLimitEnabled bool `json:"rate_limit_enabled"`
	// RateLimitRequests is the max requests per account inside the
	// sliding window. 0 disables pacing.
	RateLimitRequests int `json:"rate_limit_requests"`
	// RateLimitWindowMinutes is the sliding window length in minutes.
	RateLimitWindowMinutes int `json:"rate_limit_window_minutes"`
	// SessionStickinessEnabled keeps each API key routed to the same
	// pooled account (per model and group) for the affinity TTL so
	// subscription sessions stay on one account. It falls back
	// automatically when the pinned account is banned, cooling down, or
	// out of quota.
	SessionStickinessEnabled bool `json:"session_stickiness_enabled"`
}

var accountPoolSetting = AccountPoolSetting{
	CooldownEnabled:          true,
	CooldownMinutes:          30,
	BanIsolateEnabled:        true,
	RateLimitEnabled:         true,
	RateLimitRequests:        1200,
	RateLimitWindowMinutes:   300,
	SessionStickinessEnabled: true,
}

func init() {
	config.GlobalConfig.Register("account_pool_setting", &accountPoolSetting)
}

func GetAccountPoolSetting() *AccountPoolSetting {
	return &accountPoolSetting
}

// ApplyAccountPoolSetting copies s into the registered in-memory setting.
// Persisting the values to the options table is the caller's job.
func ApplyAccountPoolSetting(s *AccountPoolSetting) {
	if s == nil {
		return
	}
	accountPoolSetting = *s
}
