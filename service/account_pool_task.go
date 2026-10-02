package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kejiapi/common"
	"kejiapi/constant"
	"kejiapi/logger"
	"kejiapi/model"
	"kejiapi/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	accountPoolCooldownTickInterval = 60 * time.Second
	accountPoolCooldownBatchSize    = 200
)

var (
	accountPoolCooldownOnce    sync.Once
	accountPoolCooldownRunning atomic.Bool
)

// StartAccountPoolCooldownTask periodically re-enables pooled
// subscription channels that were auto-disabled with a cooldown marker
// whose deadline has passed.
func StartAccountPoolCooldownTask() {
	accountPoolCooldownOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}

		gopool.Go(func() {
			logger.LogInfo(context.Background(), "account pool cooldown recovery task started: tick=60s")

			ticker := time.NewTicker(accountPoolCooldownTickInterval)
			defer ticker.Stop()

			runAccountPoolCooldownOnce()
			for range ticker.C {
				runAccountPoolCooldownOnce()
			}
		})
	})
}

func runAccountPoolCooldownOnce() {
	if !accountPoolCooldownRunning.CompareAndSwap(false, true) {
		return
	}
	defer accountPoolCooldownRunning.Store(false)

	if !operation_setting.GetAccountPoolSetting().CooldownEnabled {
		return
	}

	ctx := context.Background()
	now := time.Now().Unix()
	restored := 0

	offset := 0
	for {
		var channels []*model.Channel
		err := model.DB.
			Select("id", "name", "status", "other").
			Where("type IN ? AND status = ? AND other LIKE ?",
				constant.SubscriptionPoolChannelTypes,
				common.ChannelStatusAutoDisabled,
				"%cooldown|%",
			).
			Order("id asc").
			Limit(accountPoolCooldownBatchSize).
			Offset(offset).
			Find(&channels).Error
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("account pool cooldown recovery: query channels failed: %v", err))
			return
		}
		if len(channels) == 0 {
			break
		}
		offset += accountPoolCooldownBatchSize

		for _, ch := range channels {
			if ch == nil {
				continue
			}
			reason, _ := ch.GetOtherInfo()["status_reason"].(string)
			deadline, ok := ParseCooldownDeadline(reason)
			if !ok || deadline > now {
				continue
			}
			if model.UpdateChannelStatus(ch.Id, "", common.ChannelStatusEnabled, "冷却结束，自动恢复") {
				restored++
				logger.LogInfo(ctx, fmt.Sprintf("account pool cooldown recovery: channel_id=%d name=%s re-enabled after cooldown", ch.Id, ch.Name))
			}
		}
	}

	if restored > 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogWarn(ctx, fmt.Sprintf("account pool cooldown recovery: InitChannelCache panic: %v", r))
				}
			}()
			model.InitChannelCache()
		}()
		logger.LogInfo(ctx, fmt.Sprintf("account pool cooldown recovery: restored=%d", restored))
	}

	// Reclaim concurrency slots leaked by transport paths without an
	// explicit release (long-lived websocket relays).
	if reaped := model.PoolReapLeaked(accountPoolLeakReapSeconds); reaped > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("account pool leak reaper: cleared=%d", reaped))
	}

	// Pause pooled accounts whose subscription window has ended.
	runAccountPoolExpiryOnce(ctx)
}

const accountPoolLeakReapSeconds = 600

// accountPoolExpiryPass pauses enabled pooled accounts whose
// "pool_expires_at" deadline passed and that opted into auto pause. Paused
// accounts are not auto-restored; an admin re-enables them manually.
func accountPoolExpiryPass(ctx context.Context) ([]int, error) {
	var channels []*model.Channel
	err := model.DB.
		Select("id", "name", "status", "other").
		Where("type IN ? AND status = ? AND other LIKE ?",
			constant.SubscriptionPoolChannelTypes,
			common.ChannelStatusEnabled,
			"%pool_expires_at%",
		).
		Order("id asc").
		Limit(accountPoolCooldownBatchSize).
		Find(&channels).Error
	if err != nil {
		return nil, err
	}
	paused := make([]int, 0, 4)
	now := time.Now().Unix()
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		expired, autoPause := accountPoolExpired(ch.GetOtherInfo(), now)
		if !expired || !autoPause {
			continue
		}
		if model.UpdateChannelStatus(ch.Id, "", common.ChannelStatusManuallyDisabled,
			"账号订阅已到期（自动停用，不自动恢复）") {
			paused = append(paused, ch.Id)
			logger.LogInfo(ctx, fmt.Sprintf("account pool expiry: channel_id=%d name=%s auto-paused (subscription expired)", ch.Id, ch.Name))
		}
	}
	return paused, nil
}

// accountPoolExpired decodes the pool expiry metadata of one channel.
// It reports whether the subscription window has ended and whether the
// account opted into automatic pause (default true).
func accountPoolExpired(otherInfo map[string]any, now int64) (bool, bool) {
	raw, ok := otherInfo["pool_expires_at"]
	if !ok {
		return false, false
	}
	var expiresAt int64
	switch v := raw.(type) {
	case float64:
		expiresAt = int64(v)
	case int64:
		expiresAt = v
	case int:
		expiresAt = int64(v)
	case string:
		if parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			expiresAt = parsed
		}
	}
	if expiresAt <= 0 {
		return false, false
	}
	autoPause := true
	if raw, ok := otherInfo["pool_auto_pause_on_expired"]; ok {
		switch v := raw.(type) {
		case bool:
			autoPause = v
		case string:
			autoPause = v == "true" || v == "1"
		}
	}
	return now >= expiresAt, autoPause
}

func runAccountPoolExpiryOnce(ctx context.Context) {
	paused, err := accountPoolExpiryPass(ctx)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("account pool expiry: query channels failed: %v", err))
		return
	}
	if len(paused) == 0 {
		return
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogWarn(ctx, fmt.Sprintf("account pool expiry: InitChannelCache panic: %v", r))
			}
		}()
		model.InitChannelCache()
	}()
}
