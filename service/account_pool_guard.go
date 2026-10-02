package service

import (
	"fmt"
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
	// them once the deadline passes. Format: 【cooldown|<unix deadline>】...
	cooldownReasonMarker = "【cooldown|"
	cooldownReasonEnd    = "】"

	// banIsolateReasonPrefix marks accounts that were isolated because the
	// upstream rejected the credential itself (banned / suspended / bad
	// key). These must NOT be auto-restored.
	banIsolateReasonPrefix = "账号已封禁（防封池自动隔离，不会自动恢复）: "
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
// that carries the cooldown marker.
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
	ts, err := strconv.ParseInt(rest[:end], 10, 64)
	if err != nil {
		return 0, false
	}
	return ts, true
}

// IsBanIsolateReason reports whether a disable reason was produced by the
// ban isolation policy.
func IsBanIsolateReason(reason string) bool {
	return strings.HasPrefix(reason, banIsolateReasonPrefix)
}

// classifyPoolError refines the disable reason for pooled subscription
// accounts: banned credentials get the isolation tag, quota / rate-limit
// errors get a timed cooldown marker. Non-pool channels and unconfigured
// policies return the reason unchanged.
func classifyPoolError(channelType int, err *types.NewAPIError, reason string) string {
	if !constant.IsSubscriptionPoolChannelType(channelType) || err == nil {
		return reason
	}
	setting := operation_setting.GetAccountPoolSetting()
	lower := strings.ToLower(err.Error())

	if setting.BanIsolateEnabled && isAccountBanError(err, lower) {
		return banIsolateReasonPrefix + reason
	}
	if setting.CooldownEnabled && isQuotaCooldownError(err, lower) {
		minutes := setting.CooldownMinutes
		if minutes <= 0 {
			minutes = 30
		}
		deadline := time.Now().Add(time.Duration(minutes) * time.Minute).Unix()
		return fmt.Sprintf("%s%d%s上游配额受限，自动冷却 %d 分钟: %s",
			cooldownReasonMarker, deadline, cooldownReasonEnd, minutes, reason)
	}
	return reason
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
