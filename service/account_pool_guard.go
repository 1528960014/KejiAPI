package service

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kejiapi/constant"
	"kejiapi/relaykit/types"
	"kejiapi/setting/operation_setting"
)

const (
	// cooldownReasonMarker is embedded into the disable reason so the
	// cooldown recovery task can locate timed-out accounts and re-enable
	// them once the deadline passes. Format: 【cooldown|<unix deadline>|<kind>】...
	// kind is one of ratelimit / overload / credential / quota.
	cooldownReasonMarker = "【cooldown|"
	cooldownReasonEnd    = "】"

	// banIsolateReasonPrefix marks accounts that were isolated because the
	// upstream rejected the credential itself (banned / suspended / bad
	// key). These must NOT be auto-restored.
	banIsolateReasonPrefix = "账号已封禁（防封池自动隔离，不会自动恢复）: "

	// poolCooldownKindRateLimit etc. classify the cooldown so the recovery
	// task and the console can tell a short 429 pause from a long overload
	// isolation or a temporary credential error.
	poolCooldownKindRateLimit  = "ratelimit"
	poolCooldownKindOverload   = "overload"
	poolCooldownKindCredential = "credential"
	poolCooldownKindQuota      = "quota"
)

var poolQuotaKeywords = []string{
	"usage limit", "usage limit reached", "rate limit", "too many requests",
	"quota", "limit reached", "resource_exhausted", "overloaded", "capacity",
	"billing", "credit", "insufficient", "payment required",
	"额度", "配额", "限流", "超限",
}

var poolBanKeywords = []string{
	"suspended", "suspension", "deactivat", "banned", "blocked",
	"terminated", "violat", "forbidden", "unauthorized",
	"invalid api key", "credential", "login required", "sign in",
	"封禁", "封号", "冻结", "停用", "注销", "违规",
}

// ParseCooldownDeadline extracts the unix deadline from a disable reason
// that carries the cooldown marker. Both the legacy two-part format
// (【cooldown|<deadline>】) and the current kind-tagged format
// (【cooldown|<deadline>|<kind>】) are understood.
func ParseCooldownDeadline(reason string) (int64, bool) {
	idx := strings.Index(reason, cooldownReasonMarker)
	if idx < 0 {
		return 0, false
	}
	rest := reason[idx+len(cooldownReasonMarker):]
	end := strings.Index(rest, cooldownReasonEnd)
	if end < 0 {
		return 0, false
	}
	deadlineField := rest[:end]
	if sep := strings.Index(deadlineField, "|"); sep >= 0 {
		deadlineField = deadlineField[:sep]
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(deadlineField), 10, 64)
	if err != nil {
		return 0, false
	}
	return ts, true
}

// ParseCooldownKind returns the cooldown classification (ratelimit /
// overload / credential / quota) of a disable reason, or "" when the
// reason carries no cooldown marker.
func ParseCooldownKind(reason string) string {
	idx := strings.Index(reason, cooldownReasonMarker)
	if idx < 0 {
		return ""
	}
	rest := reason[idx+len(cooldownReasonMarker):]
	end := strings.Index(rest, cooldownReasonEnd)
	if end < 0 {
		return ""
	}
	fields := strings.Split(rest[:end], "|")
	if len(fields) < 3 {
		return ""
	}
	return fields[2]
}

// poolCooldownDuration resolves the cooldown duration for one classified
// pool error. Durations intentionally differ by signal: a 429 window is
// short, an overload or credential error buys the upstream time to recover.
func poolCooldownDuration(kind string) time.Duration {
	setting := operation_setting.GetAccountPoolSetting()
	switch kind {
	case poolCooldownKindRateLimit:
		seconds := setting.RateLimitCooldownSeconds
		if seconds <= 0 {
			seconds = 60
		}
		return time.Duration(seconds) * time.Second
	case poolCooldownKindOverload:
		minutes := setting.OverloadCooldownMinutes
		if minutes <= 0 {
			minutes = 10
		}
		return time.Duration(minutes) * time.Minute
	case poolCooldownKindCredential:
		minutes := setting.CredentialCooldownMinutes
		if minutes <= 0 {
			minutes = 10
		}
		return time.Duration(minutes) * time.Minute
	default:
		minutes := setting.CooldownMinutes
		if minutes <= 0 {
			minutes = 30
		}
		return time.Duration(minutes) * time.Minute
	}
}

// poolCooldownLabel is the human-readable part of the cooldown marker.
func poolCooldownLabel(kind string) string {
	switch kind {
	case poolCooldownKindRateLimit:
		return "上游限流"
	case poolCooldownKindOverload:
		return "上游过载"
	case poolCooldownKindCredential:
		return "凭据暂时失效"
	default:
		return "上游配额受限"
	}
}

// IsBanIsolateReason reports whether a disable reason was produced by the
// ban isolation policy.
func IsBanIsolateReason(reason string) bool {
	return strings.HasPrefix(reason, banIsolateReasonPrefix)
}

// classifyPoolError refines the disable reason for pooled subscription
// accounts: banned credentials get the isolation tag, upstream failures
// get a timed cooldown marker whose duration follows the failure signal
// (429 seconds, 529 minutes, credential minutes, quota minutes). Non-pool
// channels and unconfigured policies return the reason unchanged.
func classifyPoolError(channelType int, err *types.NewAPIError, reason string) string {
	if !constant.IsSubscriptionPoolChannelType(channelType) || err == nil {
		return reason
	}
	setting := operation_setting.GetAccountPoolSetting()
	lower := strings.ToLower(err.Error())

	if setting.BanIsolateEnabled && isAccountBanError(err, lower) {
		return banIsolateReasonPrefix + reason
	}
	kind := poolCooldownKindFor(err, lower)
	if setting.CooldownEnabled && kind != "" {
		duration := poolCooldownDuration(kind)
		deadline := time.Now().Add(duration).Unix()
		return fmt.Sprintf("%s%d|%s%s%s%s: %s",
			cooldownReasonMarker, deadline, kind, cooldownReasonEnd,
			poolCooldownLabel(kind), duration.String(), reason)
	}
	return reason
}

// poolCooldownKindFor classifies an upstream error into a cooldown kind.
// The empty string means "do not cool down this account".
func poolCooldownKindFor(err *types.NewAPIError, lower string) string {
	switch {
	case err.StatusCode == http.StatusTooManyRequests:
		return poolCooldownKindRateLimit
	case err.StatusCode == 529:
		return poolCooldownKindOverload
	case err.StatusCode == 401 || err.StatusCode == 403:
		// 401 / 403: ban keywords were handled by the isolation policy
		// above; everything else is treated as a temporary credential
		// problem (locked session, transient auth rejection).
		return poolCooldownKindCredential
	case isQuotaCooldownError(err, lower):
		return poolCooldownKindQuota
	default:
		return ""
	}
}

func isAccountBanError(err *types.NewAPIError, lower string) bool {
	if err.StatusCode != 401 && err.StatusCode != 403 {
		return false
	}
	for _, kw := range poolBanKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func isQuotaCooldownError(err *types.NewAPIError, lower string) bool {
	if err.StatusCode == 429 {
		return true
	}
	for _, kw := range poolQuotaKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
