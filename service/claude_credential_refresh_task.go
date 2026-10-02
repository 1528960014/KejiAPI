package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kejiapi/common"
	"kejiapi/constant"
	"kejiapi/logger"
	"kejiapi/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	claudeCredentialRefreshTickInterval = 30 * time.Minute
	claudeCredentialRefreshThreshold    = 12 * time.Hour
	claudeCredentialRefreshBatchSize    = 200
	claudeCredentialRefreshTimeout      = 20 * time.Second
)

var (
	claudeCredentialRefreshOnce    sync.Once
	claudeCredentialRefreshRunning atomic.Bool
)

// StartClaudeCredentialAutoRefreshTask periodically refreshes subscription
// OAuth tokens on Anthropic channels whose credentials are JSON objects.
func StartClaudeCredentialAutoRefreshTask() {
	claudeCredentialRefreshOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}

		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("claude subscription credential auto-refresh task started: tick=%s threshold=%s", claudeCredentialRefreshTickInterval, claudeCredentialRefreshThreshold))

			ticker := time.NewTicker(claudeCredentialRefreshTickInterval)
			defer ticker.Stop()

			runClaudeCredentialAutoRefreshOnce()
			for range ticker.C {
				runClaudeCredentialAutoRefreshOnce()
			}
		})
	})
}

func runClaudeCredentialAutoRefreshOnce() {
	if !claudeCredentialRefreshRunning.CompareAndSwap(false, true) {
		return
	}
	defer claudeCredentialRefreshRunning.Store(false)

	ctx := context.Background()
	now := time.Now()

	var refreshed int
	var scanned int

	offset := 0
	for {
		var channels []*model.Channel
		err := model.DB.
			Select("id", "name", "key", "status", "channel_info").
			Where("type = ? AND (status = ? OR status = ?)",
				constant.ChannelTypeAnthropic,
				common.ChannelStatusEnabled,
				common.ChannelStatusAutoDisabled,
			).
			Order("id asc").
			Limit(claudeCredentialRefreshBatchSize).
			Offset(offset).
			Find(&channels).Error
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("claude subscription auto-refresh: query channels failed: %v", err))
			return
		}
		if len(channels) == 0 {
			break
		}
		offset += claudeCredentialRefreshBatchSize

		for _, ch := range channels {
			if ch == nil {
				continue
			}
			scanned++
			if ch.ChannelInfo.IsMultiKey {
				continue
			}

			rawKey := strings.TrimSpace(ch.Key)
			if rawKey == "" || !strings.HasPrefix(rawKey, "{") {
				continue
			}

			oauthKey, err := parseClaudeOAuthKey(rawKey)
			if err != nil {
				continue
			}
			if strings.TrimSpace(oauthKey.RefreshToken) == "" {
				continue
			}

			expiredAt, err := time.Parse(time.RFC3339, strings.TrimSpace(oauthKey.Expired))
			if err == nil && !expiredAt.IsZero() && expiredAt.Sub(now) > claudeCredentialRefreshThreshold {
				continue
			}

			refreshCtx, cancel := context.WithTimeout(ctx, claudeCredentialRefreshTimeout)
			newKey, _, err := RefreshClaudeChannelCredential(refreshCtx, ch.Id, ClaudeCredentialRefreshOptions{ResetCaches: false})
			cancel()
			if err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("claude subscription auto-refresh: channel_id=%d name=%s refresh failed: %v", ch.Id, ch.Name, err))
				continue
			}

			refreshed++
			logger.LogInfo(ctx, fmt.Sprintf("claude subscription auto-refresh: channel_id=%d name=%s refreshed, expires_at=%s", ch.Id, ch.Name, newKey.Expired))
		}
	}

	if refreshed > 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogWarn(ctx, fmt.Sprintf("claude subscription auto-refresh: InitChannelCache panic: %v", r))
				}
			}()
			model.InitChannelCache()
		}()
	}

	if common.DebugEnabled {
		logger.LogDebug(ctx, "claude subscription auto-refresh: scanned=%d refreshed=%d", scanned, refreshed)
	}
}
