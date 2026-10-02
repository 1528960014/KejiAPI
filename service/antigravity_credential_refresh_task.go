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
	antigravityCredentialRefreshTickInterval = 30 * time.Minute
	antigravityCredentialRefreshThreshold    = 45 * time.Minute
	antigravityCredentialRefreshBatchSize    = 200
	antigravityCredentialRefreshTimeout      = 20 * time.Second
)

var (
	antigravityCredentialRefreshOnce    sync.Once
	antigravityCredentialRefreshRunning atomic.Bool
)

// StartAntigravityCredentialAutoRefreshTask periodically refreshes Google
// OAuth tokens on Antigravity channels whose credentials are JSON objects.
// Google access tokens are short-lived (~1 hour), so the threshold is small.
func StartAntigravityCredentialAutoRefreshTask() {
	antigravityCredentialRefreshOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}

		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("antigravity subscription credential auto-refresh task started: tick=%s threshold=%s", antigravityCredentialRefreshTickInterval, antigravityCredentialRefreshThreshold))

			ticker := time.NewTicker(antigravityCredentialRefreshTickInterval)
			defer ticker.Stop()

			runAntigravityCredentialAutoRefreshOnce()
			for range ticker.C {
				runAntigravityCredentialAutoRefreshOnce()
			}
		})
	})
}

func runAntigravityCredentialAutoRefreshOnce() {
	if !antigravityCredentialRefreshRunning.CompareAndSwap(false, true) {
		return
	}
	defer antigravityCredentialRefreshRunning.Store(false)

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
				constant.ChannelTypeAntigravity,
				common.ChannelStatusEnabled,
				common.ChannelStatusAutoDisabled,
			).
			Order("id asc").
			Limit(antigravityCredentialRefreshBatchSize).
			Offset(offset).
			Find(&channels).Error
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("antigravity subscription auto-refresh: query channels failed: %v", err))
			return
		}
		if len(channels) == 0 {
			break
		}
		offset += antigravityCredentialRefreshBatchSize

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

			oauthKey, err := parseAntigravityOAuthKey(rawKey)
			if err != nil {
				continue
			}
			if strings.TrimSpace(oauthKey.RefreshToken) == "" {
				continue
			}

			expiredAt, err := time.Parse(time.RFC3339, strings.TrimSpace(oauthKey.Expired))
			if err == nil && !expiredAt.IsZero() && expiredAt.Sub(now) > antigravityCredentialRefreshThreshold {
				continue
			}

			refreshCtx, cancel := context.WithTimeout(ctx, antigravityCredentialRefreshTimeout)
			newKey, _, err := RefreshAntigravityChannelCredential(refreshCtx, ch.Id, AntigravityCredentialRefreshOptions{ResetCaches: false})
			cancel()
			if err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("antigravity subscription auto-refresh: channel_id=%d name=%s refresh failed: %v", ch.Id, ch.Name, err))
				continue
			}

			refreshed++
			logger.LogInfo(ctx, fmt.Sprintf("antigravity subscription auto-refresh: channel_id=%d name=%s refreshed, expires_at=%s", ch.Id, ch.Name, newKey.Expired))
		}
	}

	if refreshed > 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogWarn(ctx, fmt.Sprintf("antigravity subscription auto-refresh: InitChannelCache panic: %v", r))
				}
			}()
			model.InitChannelCache()
		}()
	}

	if common.DebugEnabled {
		logger.LogDebug(ctx, "antigravity subscription auto-refresh: scanned=%d refreshed=%d", scanned, refreshed)
	}
}