package controller

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"kejiapi/common"
	"kejiapi/constant"
	"kejiapi/model"
	"kejiapi/relaykit/types"
	"kejiapi/service"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const (
	// poolHealthTickInterval is how often the scheduled health checker
	// looks for pool accounts whose check is due.
	poolHealthTickInterval = 60 * time.Second
	// poolHealthBatchSize bounds one scan of the pool table.
	poolHealthBatchSize = 200
	// poolHealthResultsKeep is the number of recent outcomes stored on
	// each account for the console.
	poolHealthResultsKeep = 5
	// poolHealthDefaultIntervalMinutes is used when an account enables
	// scheduled checks without an explicit interval.
	poolHealthDefaultIntervalMinutes = 60
	// poolHealthErrorPreview caps the stored error text.
	poolHealthErrorPreview = 200
)

var (
	accountPoolHealthOnce    sync.Once
	accountPoolHealthRunning atomic.Bool
)

// StartAccountPoolHealthTask periodically runs the scheduled health checks
// of pool accounts that opted in via "pool_health_enabled". A successful
// check clears pool-generated isolation / cooldown markers and re-enables
// the account; a failing one disables it when the account allows automatic
// disabling, mirroring the per-account scheduled tests of a full account
// pool deployment.
func StartAccountPoolHealthTask() {
	accountPoolHealthOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}

		gopool.Go(func() {
			logLine := "account pool health check task started: tick=60s"
			common.SysLog(logLine)

			ticker := time.NewTicker(poolHealthTickInterval)
			defer ticker.Stop()

			runAccountPoolHealthOnceWithReport(nil)
			for range ticker.C {
				runAccountPoolHealthOnceWithReport(nil)
			}
		})
	})
}

func runAccountPoolHealthOnceWithReport(report func(processed, total int)) {
	if !accountPoolHealthRunning.CompareAndSwap(false, true) {
		return
	}
	defer accountPoolHealthRunning.Store(false)

	ctx := context.Background()
	channels, err := loadPoolHealthChannels()
	if err != nil {
		common.SysLog(fmt.Sprintf("account pool health check: load channels failed: %v", err))
		return
	}
	if len(channels) == 0 {
		return
	}

	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return
	}

	total := 0
	for _, channel := range channels {
		if poolHealthDue(channel, time.Now().Unix()) {
			total++
		}
	}
	if total == 0 {
		return
	}
	if report != nil {
		report(0, total)
	}

	processed := 0
	for _, channel := range channels {
		if !poolHealthDue(channel, time.Now().Unix()) {
			continue
		}
		runPoolHealthCheck(ctx, channel, testUserID)
		processed++
		if report != nil {
			report(processed, total)
		}
	}
}

func loadPoolHealthChannels() ([]*model.Channel, error) {
	var channels []*model.Channel
	err := model.DB.
		Select("id", "name", "type", "status", "other", "auto_ban", "models", "group").
		Where("type IN ? AND other LIKE ?",
			constant.SubscriptionPoolChannelTypes,
			"pool_health%").
		Order("id asc").
		Limit(poolHealthBatchSize).
		Find(&channels).Error
	return channels, err
}

// poolHealthDue reports whether the account's scheduled check is enabled
// and its interval has elapsed.
func poolHealthDue(channel *model.Channel, now int64) bool {
	options := poolHealthOptions(channel)
	if !options.enabled {
		return false
	}
	interval := options.intervalMinutes
	return now >= options.lastRunAt+int64(interval)*60
}

// poolHealthOptions decodes the per-account scheduled check config from the
// channel extra metadata.
type poolHealthConfig struct {
	enabled         bool
	intervalMinutes int
	model           string
	lastRunAt       int64
	results         []any
}

func poolHealthOptions(channel *model.Channel) poolHealthConfig {
	config := poolHealthConfig{}
	if channel == nil {
		return config
	}
	info := channel.GetOtherInfo()
	if v, ok := info["pool_health_enabled"].(bool); ok {
		config.enabled = v
	} else if v, ok := info["pool_health_enabled"].(string); ok {
		config.enabled = v == "true" || v == "1"
	}
	config.intervalMinutes = poolHealthDefaultIntervalMinutes
	switch v := info["pool_health_interval_minutes"].(type) {
	case float64:
		config.intervalMinutes = int(v)
	case int:
		config.intervalMinutes = v
	}
	if config.intervalMinutes < 1 {
		config.intervalMinutes = 1
	}
	if v, ok := info["pool_health_model"].(string); ok {
		config.model = v
	}
	switch v := info["pool_health_last_at"].(type) {
	case float64:
		config.lastRunAt = int64(v)
	case int64:
		config.lastRunAt = v
	}
	if v, ok := info["pool_health_results"].([]any); ok {
		config.results = v
	}
	return config
}

// runPoolHealthCheck runs one scheduled test for a pool account, persists
// the outcome and applies the recovery / disable policy.
func runPoolHealthCheck(ctx context.Context, channel *model.Channel, testUserID int) {
	if channel == nil {
		return
	}
	options := poolHealthOptions(channel)
	start := time.Now()
	result := testChannel(ctx, channel, testUserID, options.model, "", shouldUseStreamForAutomaticChannelTest(channel))
	latencyMs := time.Since(start).Milliseconds()
	now := time.Now().Unix()

	succeeded := result.localErr == nil && result.newAPIError == nil
	errText := ""
	if result.localErr != nil {
		errText = result.localErr.Error()
	} else if result.newAPIError != nil {
		errText = result.newAPIError.Error()
	}

	if succeeded {
		// Recovery: clear pool-generated markers and re-enable the account
		// so the scheduler can select it again.
		cleared := clearPoolErrorMarkers(channel)
		if channel.Status != common.ChannelStatusEnabled && service.ShouldEnableChannel(nil, channel.Status) {
			service.EnableChannel(channel.Id, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.Name)
		} else if cleared {
			if err := model.DB.Model(channel).Update("other", channel.OtherInfo).Error; err != nil {
				common.SysLog(fmt.Sprintf("account pool health check: channel_id=%d persist cleared markers failed: %v", channel.Id, err))
			}
		}
	} else if channel.Status == common.ChannelStatusEnabled && channel.GetAutoBan() && service.ShouldDisableChannel(result.newAPIError) {
		reason := fmt.Sprintf("账号健康检查失败: %s", errText)
		service.DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.GetAutoBan()), reason)
	}

	appendPoolHealthResult(channel, succeeded, latencyMs, errText, now)
	if err := model.DB.Model(channel).Update("other", channel.OtherInfo).Error; err != nil {
		common.SysLog(fmt.Sprintf("account pool health check: channel_id=%d persist result failed: %v", channel.Id, err))
	}

	status := "ok"
	if !succeeded {
		status = "fail"
	}
	common.SysLog(fmt.Sprintf("account pool health check: channel_id=%d name=%s status=%s latency=%dms", channel.Id, channel.Name, status, latencyMs))
}

// clearPoolErrorMarkers strips pool-generated disable reasons (cooldown /
// isolation markers) from the channel metadata. It reports whether
// anything was removed.
func clearPoolErrorMarkers(channel *model.Channel) bool {
	info := channel.GetOtherInfo()
	reason, _ := info["status_reason"].(string)
	if reason == "" || !service.PoolErrorReasonIsAutoGenerated(reason) {
		return false
	}
	delete(info, "status_reason")
	channel.SetOtherInfo(info)
	return true
}

// appendPoolHealthResult records one outcome in the channel metadata and
// keeps only the most recent poolHealthResultsKeep entries.
func appendPoolHealthResult(channel *model.Channel, succeeded bool, latencyMs int64, errText string, now int64) {
	info := channel.GetOtherInfo()
	results := []any{}
	if existing, ok := info["pool_health_results"].([]any); ok {
		results = append(results, existing...)
	}
	if len(errText) > poolHealthErrorPreview {
		errText = errText[:poolHealthErrorPreview]
	}
	results = append([]any{gin.H{
		"at":         now,
		"ok":         succeeded,
		"latency_ms": latencyMs,
		"error":      errText,
	}}, results...)
	if len(results) > poolHealthResultsKeep {
		results = results[:poolHealthResultsKeep]
	}
	info["pool_health_results"] = results
	info["pool_health_last_at"] = now
	channel.SetOtherInfo(info)
}

// PoolHealthTestChannel runs one health check on demand.
//
// POST /api/channel/pool/health-test {channel_id, model}
func PoolHealthTestChannel(c *gin.Context) {
	var req struct {
		ChannelId int    `json:"channel_id"`
		Model     string `json:"model"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.ChannelId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	channel, err := model.CacheGetChannel(req.ChannelId)
	if err != nil || channel == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "账号不存在"})
		return
	}
	if !constant.IsSubscriptionPoolChannelType(channel.Type) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "该账号不属于订阅池类型"})
		return
	}
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无可用的测试用户"})
		return
	}

	modelName := req.Model
	if modelName == "" {
		modelName = poolHealthOptions(channel).model
	}
	start := time.Now()
	result := testChannel(c.Request.Context(), channel, testUserID, modelName, "", shouldUseStreamForAutomaticChannelTest(channel))
	latencyMs := time.Since(start).Milliseconds()
	succeeded := result.localErr == nil && result.newAPIError == nil
	errText := ""
	if result.localErr != nil {
		errText = result.localErr.Error()
	} else if result.newAPIError != nil {
		errText = result.newAPIError.Error()
	}

	appendPoolHealthResult(channel, succeeded, latencyMs, errText, time.Now().Unix())
	if err := model.DB.Model(channel).Update("other", channel.OtherInfo).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存检查结果失败: " + err.Error()})
		return
	}
	if succeeded {
		clearPoolErrorMarkers(channel)
		if channel.Status != common.ChannelStatusEnabled && service.ShouldEnableChannel(nil, channel.Status) {
			service.EnableChannel(channel.Id, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.Name)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"channel_id": channel.Id,
			"ok":         succeeded,
			"latency_ms": latencyMs,
			"error":      errText,
		},
	})
}
