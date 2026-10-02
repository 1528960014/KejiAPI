package controller

import (
	"net/http"
	"strings"

	"kejiapi/common"
	"kejiapi/constant"
	"kejiapi/model"
	"kejiapi/service"
	"kejiapi/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// GetChannelAccountPool returns the aggregated account pool view: every
// pooled subscription / gateway account channel with credential status,
// ban / cooldown state, pool statistics and the current policy settings.
//
// GET /api/channel/pool
func GetChannelAccountPool(c *gin.Context) {
	var channels []*model.Channel
	if err := model.DB.Where("type IN ?", constant.SubscriptionPoolChannelTypes).
		Order("type asc, id asc").
		Find(&channels).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	accounts := make([]gin.H, 0, len(channels))
	statsByType := make(map[int]int)
	statusCounts := map[string]int{"enabled": 0, "auto_disabled": 0, "manually_disabled": 0}
	cooldownCount := 0
	bannedCount := 0

	for _, ch := range channels {
		if ch == nil {
			continue
		}
		reason, _ := ch.GetOtherInfo()["status_reason"].(string)
		cooldownDeadline := int64(0)
		cooldownKind := ""
		if deadline, ok := service.ParseCooldownDeadline(reason); ok {
			cooldownDeadline = deadline
			cooldownCount++
		}
		cooldownKind = service.ParseCooldownKind(reason)
		banned := service.IsBanIsolateReason(reason)
		if banned {
			bannedCount++
		}

		switch ch.Status {
		case common.ChannelStatusEnabled:
			statusCounts["enabled"]++
		case common.ChannelStatusAutoDisabled:
			statusCounts["auto_disabled"]++
		case common.ChannelStatusManuallyDisabled:
			statusCounts["manually_disabled"]++
		}

		credential := gin.H{}
		rawKey := strings.TrimSpace(ch.Key)
		if strings.HasPrefix(rawKey, "{") {
			if status, err := subscriptionCredentialStatusFromKey(ch.Type, rawKey); err == nil {
				credential = status
			}
		}

		otherInfo := ch.GetOtherInfo()
		maxConcurrency := 0
		switch v := otherInfo["pool_max_concurrency"].(type) {
		case float64:
			maxConcurrency = int(v)
		case int:
			maxConcurrency = v
		}
		expiresAt := int64(0)
		switch v := otherInfo["pool_expires_at"].(type) {
		case float64:
			expiresAt = int64(v)
		case int64:
			expiresAt = v
		}
		autoPause := true
		if v, ok := otherInfo["pool_auto_pause_on_expired"].(bool); ok {
			autoPause = v
		}

		accounts = append(accounts, gin.H{
			"id":                ch.Id,
			"name":              ch.Name,
			"type":              ch.Type,
			"status":            ch.Status,
			"status_reason":     reason,
			"models":            ch.Models,
			"group":             ch.Group,
			"priority":          ch.Priority,
			"weight":            ch.Weight,
			"auto_ban":          ch.GetAutoBan(),
			"created_time":      ch.CreatedTime,
			"cooldown_deadline": cooldownDeadline,
			"cooldown_kind":     cooldownKind,
			"banned":            banned,
			"max_concurrency":   maxConcurrency,
			"expires_at":        expiresAt,
			"auto_pause":        autoPause,
			"pool_runtime":      model.PoolSnapshot(ch.Id),
			"credential":        credential,
		})
		statsByType[ch.Type]++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"accounts": accounts,
			"stats": gin.H{
				"total":     len(accounts),
				"by_type":   statsByType,
				"by_status": statusCounts,
				"cooldown":  cooldownCount,
				"banned":    bannedCount,
			},
			"settings": operation_setting.GetAccountPoolSetting(),
		},
	})
}

// UpdateChannelAccountPoolSettings updates the account pool ban-prevention
// policy (cooldown, ban isolation, per-account pacing).
//
// PUT /api/channel/pool/settings
func UpdateChannelAccountPoolSettings(c *gin.Context) {
	current := operation_setting.GetAccountPoolSetting()
	s := *current
	if err := common.DecodeJson(c.Request.Body, &s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if s.CooldownMinutes < 1 {
		s.CooldownMinutes = 30
	}
	if s.CooldownMinutes > 24*60 {
		s.CooldownMinutes = 24 * 60
	}
	if s.RateLimitCooldownSeconds < 5 {
		s.RateLimitCooldownSeconds = 5
	}
	if s.RateLimitCooldownSeconds > 24*60*60 {
		s.RateLimitCooldownSeconds = 24 * 60 * 60
	}
	if s.OverloadCooldownMinutes < 1 {
		s.OverloadCooldownMinutes = 10
	}
	if s.OverloadCooldownMinutes > 24*60 {
		s.OverloadCooldownMinutes = 24 * 60
	}
	if s.CredentialCooldownMinutes < 1 {
		s.CredentialCooldownMinutes = 10
	}
	if s.CredentialCooldownMinutes > 24*60 {
		s.CredentialCooldownMinutes = 24 * 60
	}
	if s.SelectionTopK < 0 {
		s.SelectionTopK = 0
	}
	if s.SelectionTopK > 32 {
		s.SelectionTopK = 32
	}
	if s.RateLimitWindowMinutes < 1 {
		s.RateLimitWindowMinutes = 300
	}
	if s.RateLimitWindowMinutes > 24*60 {
		s.RateLimitWindowMinutes = 24 * 60
	}
	if s.RateLimitRequests < 0 {
		s.RateLimitRequests = 0
	}
	if s.RateLimitRequests > 1000000 {
		s.RateLimitRequests = 1000000
	}

	updates := map[string]string{
		"account_pool_setting.cooldown_enabled":            common.Interface2String(s.CooldownEnabled),
		"account_pool_setting.cooldown_minutes":            common.Interface2String(s.CooldownMinutes),
		"account_pool_setting.rate_limit_cooldown_seconds": common.Interface2String(s.RateLimitCooldownSeconds),
		"account_pool_setting.overload_cooldown_minutes":   common.Interface2String(s.OverloadCooldownMinutes),
		"account_pool_setting.credential_cooldown_minutes": common.Interface2String(s.CredentialCooldownMinutes),
		"account_pool_setting.ban_isolate_enabled":         common.Interface2String(s.BanIsolateEnabled),
		"account_pool_setting.rate_limit_enabled":          common.Interface2String(s.RateLimitEnabled),
		"account_pool_setting.rate_limit_requests":         common.Interface2String(s.RateLimitRequests),
		"account_pool_setting.rate_limit_window_minutes":   common.Interface2String(s.RateLimitWindowMinutes),
		"account_pool_setting.session_stickiness_enabled":  common.Interface2String(s.SessionStickinessEnabled),
		"account_pool_setting.selection_top_k":             common.Interface2String(s.SelectionTopK),
	}
	for key, value := range updates {
		if err := model.UpdateOption(key, value); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	operation_setting.ApplyAccountPoolSetting(&s)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": s})
}
