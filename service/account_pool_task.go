package service

import (
	"context"
	"fmt"
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
}
