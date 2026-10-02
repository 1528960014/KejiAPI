package operation_setting

import "kejiapi/setting/config"

// AccountPoolSetting controls the account pool management policy:
// session-aware scheduling (stickiness, load balancing, per-account
// concurrency), signal-aware cooldowns (rate limit / overload / credential
// errors), and the ban-prevention policy (isolation, per-account pacing).
type AccountPoolSetting struct {
	// CooldownEnabled automatically disables (with a timed marker)
	// subscription channels that hit upstream quota / rate-limit errors,
	// and re-enables them after CooldownMinutes.
	CooldownEnabled bool `json:"cooldown_enabled"`
	// CooldownMinutes is the default cooldown duration in minutes for
	// generic upstream quota errors.
	CooldownMinutes int `json:"cooldown_minutes"`
	// RateLimitCooldownSeconds is the cooldown applied to HTTP 429
	// (upstream rate limit). It is intentionally short because 429 windows
	// are usually seconds to a few minutes.
	RateLimitCooldownSeconds int `json:"rate_limit_cooldown_seconds"`
	// OverloadCooldownMinutes is the isolation time applied to HTTP 529
	// (upstream overload).
	OverloadCooldownMinutes int `json:"overload_cooldown_minutes"`
	// CredentialCooldownMinutes is the recovery time applied to temporary
	// credential errors (401 / 403 without an explicit ban signal).
	CredentialCooldownMinutes int `json:"credential_cooldown_minutes"`
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
	// automatically when the pinned account is banned, cooling down,
	// saturated or unhealthy.
	SessionStickinessEnabled bool `json:"session_stickiness_enabled"`
	// SelectionTopK is the number of healthiest pooled accounts that
	// participate in each weighted draw. Lower values concentrate traffic
	// on the healthiest accounts; 0 disables load-aware ordering.
	SelectionTopK int `json:"selection_top_k"`
}

var accountPoolSetting = AccountPoolSetting{
	CooldownEnabled:           true,
	CooldownMinutes:           30,
	RateLimitCooldownSeconds:  60,
	OverloadCooldownMinutes:   10,
	CredentialCooldownMinutes: 10,
	BanIsolateEnabled:         true,
	RateLimitEnabled:          true,
	RateLimitRequests:         1200,
	RateLimitWindowMinutes:    300,
	SessionStickinessEnabled:  true,
	SelectionTopK:             3,
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
