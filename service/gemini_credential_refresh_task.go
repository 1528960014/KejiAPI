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
	geminiCredentialRefreshTickInterval = 30 * time.Minute
	geminiCredentialRefreshThreshold    = 45 * time.Minute
	geminiCredentialRefreshBatchSize    = 200
	geminiCredentialRefreshTimeout      = 20 * time.Second
)

var (
	geminiCredentialRefreshOnce    sync.Once
	geminiCredentialRefreshRunning atomic.Bool
)

// StartGeminiCredentialAutoRefreshTask periodically refreshes Google OAuth
// tokens on Gemini channels whose credentials are JSON objects. Google
// access tokens are short-lived (~1 hour), so the threshold is small.
func StartGeminiCredentialAutoRefreshTask() {
	geminiCredentialRefreshOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}

		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("gemini subscription credential auto-refresh task started: tick=%s threshold=%s", geminiCredentialRefreshTickInterval, geminiCredentialRefreshThreshold))

			ticker := time.NewTicker(geminiCredentialRefreshTickInterval)
			defer ticker.Stop()

			runGeminiCredentialAutoRefreshOnce()
			for range ticker.C {
				runGeminiCredentialAutoRefreshOnce()
			}
		})
	})
}

func runGeminiCredentialAutoRefreshOnce() {
	if !geminiCredentialRefreshRunning.CompareAndSwap(false, true) {
		return
	}
	defer geminiCredentialRefreshRunning.Store(false)

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
				constant.ChannelTypeGemini,
				common.ChannelStatusEnabled,
				common.ChannelStatusAutoDisabled,
			).
			Order("id asc").
			Limit(geminiCredentialRefreshBatchSize).
			Offset(offset).
			Find(&channels).Error
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("gemini subscription auto-refresh: query channels failed: %v", err))
			return
		}
		if len(channels) == 0 {
			break
		}
		offset += geminiCredentialRefreshBatchSize

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

			oauthKey, err := parseGeminiOAuthKey(rawKey)
			if err != nil {
				continue
			}
			if strings.TrimSpace(oauthKey.RefreshToken) == "" {
				continue
			}

			expiredAt, err := time.Parse(time.RFC3339, strings.TrimSpace(oauthKey.Expired))
			if err == nil && !expiredAt.IsZero() && expiredAt.Sub(now) > geminiCredentialRefreshThreshold {
				continue
			}

			refreshCtx, cancel := context.WithTimeout(ctx, geminiCredentialRefreshTimeout)
			newKey, _, err := RefreshGeminiChannelCredential(refreshCtx, ch.Id, GeminiCredentialRefreshOptions{ResetCaches: false})
			cancel()
			if err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("gemini subscription auto-refresh: channel_id=%d name=%s refresh failed: %v", ch.Id, ch.Name, err))
				continue
			}

			refreshed++
			logger.LogInfo(ctx, fmt.Sprintf("gemini subscription auto-refresh: channel_id=%d name=%s refreshed, expires_at=%s", ch.Id, ch.Name, newKey.Expired))
		}
	}

	if refreshed > 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogWarn(ctx, fmt.Sprintf("gemini subscription auto-refresh: InitChannelCache panic: %v", r))
				}
			}()
			model.InitChannelCache()
		}()
	}

	if common.DebugEnabled {
		logger.LogDebug(ctx, "gemini subscription auto-refresh: scanned=%d refreshed=%d", scanned, refreshed)
	}
}
