package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kejiapi/common"
	"kejiapi/constant"
	"kejiapi/model"
	"kejiapi/service"

	"github.com/gin-gonic/gin"
)

const (
	subscriptionProviderClaude      = "claude"
	subscriptionProviderGPT         = "gpt"
	subscriptionProviderGemini      = "gemini"
	subscriptionProviderAntigravity = "antigravity"

	defaultClaudeSubscriptionModels = "claude-sonnet-4-5,claude-opus-4-1,claude-haiku-4-5,claude-sonnet-4-0,claude-opus-4-0,claude-3-7-sonnet-latest,claude-3-5-sonnet-latest,claude-3-5-haiku-latest"
	defaultGPTSubscriptionModels    = "gpt-5-codex,gpt-5-codex-mini,gpt-5.1-codex-max,gpt-5,gpt-5-mini,gpt-5-nano,gpt-4o,gpt-4o-mini"
	defaultGeminiSubscriptionModels = "gemini-2.5-pro,gemini-2.5-flash,gemini-2.5-flash-lite,gemini-2.0-flash,gemini-2.0-flash-lite"

	defaultAntigravitySubscriptionModels = "claude-sonnet-4-6,claude-opus-4-6-thinking,gemini-3-pro-high,gemini-3-pro-low,gpt-oss-120b-medium"
)

// GetChannelSubscriptionAuthURL returns the provider authorization URL the
// admin must open to log in with a subscription account.
//
// POST /api/channel/subscription/auth-url  {"provider": "claude" | "gpt"}
func GetChannelSubscriptionAuthURL(c *gin.Context) {
	var req struct {
		Provider string `json:"provider"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))

	switch provider {
	case subscriptionProviderClaude:
		authURL, state, err := service.GenerateClaudeOAuthAuthURL()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"provider": provider,
				"auth_url": authURL,
				"state":    state,
			},
		})
	case subscriptionProviderGPT:
		res, err := service.GenerateGPTOAuthAuthURL()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"provider":      provider,
				"auth_url":      res.AuthURL,
				"code_verifier": res.CodeVerifier,
			},
		})
	case subscriptionProviderGemini:
		res, err := service.GenerateGeminiOAuthAuthURL()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"provider":      provider,
				"auth_url":      res.AuthURL,
				"code_verifier": res.CodeVerifier,
			},
		})
	case subscriptionProviderAntigravity:
		res, err := service.GenerateAntigravityOAuthAuthURL()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"provider":      provider,
				"auth_url":      res.AuthURL,
				"code_verifier": res.CodeVerifier,
			},
		})
	default:
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("不支持的订阅提供商: %s", provider),
		})
	}
}

type subscriptionCreateRequest struct {
	Provider     string  `json:"provider"`
	Code         string  `json:"code"`
	CodeVerifier string  `json:"code_verifier"`
	Name         string  `json:"name"`
	Group        string  `json:"group"`
	Models       string  `json:"models"`
	Weight       *uint   `json:"weight"`
	Priority     *int64  `json:"priority"`
}

// CreateSubscriptionChannel exchanges the pasted authorization code and
// creates a subscription channel (Anthropic type 14 or Codex type 57)
// whose key holds the JSON OAuth credential.
//
// POST /api/channel/subscription/create
func CreateSubscriptionChannel(c *gin.Context) {
	var req subscriptionCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if strings.TrimSpace(req.Code) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "授权码不能为空"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	chType := 0
	keyJSON := ""
	defaultModels := ""
	providerLabel := ""

	switch provider {
	case subscriptionProviderClaude:
		chType = constant.ChannelTypeAnthropic
		defaultModels = defaultClaudeSubscriptionModels
		providerLabel = "Claude"

		cred, err := service.ExchangeClaudeOAuthCode(ctx, req.Code, "")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "授权码兑换失败: " + err.Error()})
			return
		}
		if b, err := common.Marshal(cred); err == nil {
			keyJSON = string(b)
		} else {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.Set("subscription_email", cred.Email)
	case subscriptionProviderGPT:
		chType = constant.ChannelTypeCodex
		defaultModels = defaultGPTSubscriptionModels
		providerLabel = "ChatGPT"

		cred, err := service.ExchangeGPTOAuthCode(ctx, req.Code, req.CodeVerifier, "")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "授权码兑换失败: " + err.Error()})
			return
		}
		if b, err := common.Marshal(cred); err == nil {
			keyJSON = string(b)
		} else {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.Set("subscription_email", cred.Email)
	case subscriptionProviderGemini:
		chType = constant.ChannelTypeGemini
		defaultModels = defaultGeminiSubscriptionModels
		providerLabel = "Gemini"

		cred, err := service.ExchangeGeminiOAuthCode(ctx, req.Code, req.CodeVerifier, "")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "授权码兑换失败: " + err.Error()})
			return
		}
		if b, err := common.Marshal(cred); err == nil {
			keyJSON = string(b)
		} else {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.Set("subscription_email", cred.Email)
	case subscriptionProviderAntigravity:
		chType = constant.ChannelTypeAntigravity
		defaultModels = defaultAntigravitySubscriptionModels
		providerLabel = "Antigravity"

		// Exchange may trigger a multi-step onboarding flow (project
		// discovery), which can take up to a minute.
		agyCtx, agyCancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
		cred, err := service.ExchangeAntigravityOAuthCode(agyCtx, req.Code, req.CodeVerifier, "")
		if err == nil {
			err = service.DiscoverAntigravityProject(agyCtx, cred, "")
			if err == nil && strings.TrimSpace(cred.ProjectID) == "" {
				err = errors.New("未能获取 Google Cloud 项目 ID，请确认账号已开通 AI 订阅")
			}
		}
		agyCancel()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "授权码兑换或项目发现失败: " + err.Error()})
			return
		}
		if b, err := common.Marshal(cred); err == nil {
			keyJSON = string(b)
		} else {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.Set("subscription_email", cred.Email)
	default:
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("不支持的订阅提供商: %s", provider),
		})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = buildSubscriptionChannelName(ctx, chType, providerLabel)
	}
	if email, ok := c.Get("subscription_email"); ok {
		if s, isStr := email.(string); isStr && strings.TrimSpace(s) != "" {
			name = fmt.Sprintf("%s (%s)", providerLabel, s)
		}
	}

	group := strings.TrimSpace(req.Group)
	if group == "" {
		group = "default"
	}
	models := strings.TrimSpace(req.Models)
	if models == "" {
		models = defaultModels
	}

	var weight uint
	if req.Weight != nil {
		weight = *req.Weight
	} else {
		weight = 10
	}
	var priority int64
	if req.Priority != nil {
		priority = *req.Priority
	}

	baseURL := constant.GetChannelBaseURL(chType)
	channel := model.Channel{
		Type:        chType,
		Key:         keyJSON,
		Status:      common.ChannelStatusEnabled,
		Name:        name,
		Weight:      &weight,
		BaseURL:     &baseURL,
		Models:      models,
		Group:       group,
		Priority:    &priority,
		CreatedTime: common.GetTimestamp(),
	}

	if err := model.BatchInsertChannels([]model.Channel{channel}); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "渠道创建失败: " + err.Error()})
		return
	}
	// Batch insert does not write the generated id back; resolve it.
	if channel.Id == 0 {
		_ = model.DB.Where("name = ? AND type = ?", name, chType).Order("id desc").First(&channel).Error
	}
	model.InitChannelCache()

	recordManageAudit(c, "channel.subscription_create", map[string]any{
		"provider": provider,
		"name":     name,
		"id":       channel.Id,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":   channel.Id,
			"name": channel.Name,
		},
	})
}

func buildSubscriptionChannelName(ctx context.Context, chType int, providerLabel string) string {
	var count int64
	_ = model.DB.Model(&model.Channel{}).Where("type = ?", chType).Count(&count).Error
	return fmt.Sprintf("%s 订阅 #%d", providerLabel, count+1)
}

// RefreshSubscriptionChannelCredential refreshes the OAuth token of an
// existing subscription channel.
//
// POST /api/channel/:id/subscription/refresh
func RefreshSubscriptionChannelCredential(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	ch, err := model.GetChannelById(id, true)
	if err != nil || ch == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "渠道不存在"})
		return
	}

	switch ch.Type {
	case constant.ChannelTypeAnthropic:
		key, _, err := service.RefreshClaudeChannelCredential(ctx, id, service.ClaudeCredentialRefreshOptions{ResetCaches: true})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    subscriptionCredentialStatus(ch.Type, keyJSONValue(key.AccessToken, key.RefreshToken, key.Expired, key.LastRefresh, key.AccountUUID, key.Email)),
		})
	case constant.ChannelTypeCodex:
		key, _, err := service.RefreshCodexChannelCredential(ctx, id, service.CodexCredentialRefreshOptions{ResetCaches: true})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    subscriptionCredentialStatus(ch.Type, keyJSONValue(key.AccessToken, key.RefreshToken, key.Expired, key.LastRefresh, key.AccountID, key.Email)),
		})
	case constant.ChannelTypeGemini:
		key, _, err := service.RefreshGeminiChannelCredential(ctx, id, service.GeminiCredentialRefreshOptions{ResetCaches: true})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    subscriptionCredentialStatus(ch.Type, keyJSONValue(key.AccessToken, key.RefreshToken, key.Expired, key.LastRefresh, "", key.Email)),
		})
	case constant.ChannelTypeAntigravity:
		key, _, err := service.RefreshAntigravityChannelCredential(ctx, id, service.AntigravityCredentialRefreshOptions{ResetCaches: true})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    subscriptionCredentialStatus(ch.Type, keyJSONValue(key.AccessToken, key.RefreshToken, key.Expired, key.LastRefresh, "", key.Email)),
		})
	default:
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "该渠道类型不支持订阅凭据刷新"})
	}
}

// GetSubscriptionChannelStatus reports the token validity of a subscription
// channel without exposing the credential material.
//
// GET /api/channel/:id/subscription/status
func GetSubscriptionChannelStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ch, err := model.GetChannelById(id, true)
	if err != nil || ch == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "渠道不存在"})
		return
	}

	rawKey := strings.TrimSpace(ch.Key)
	if !strings.HasPrefix(rawKey, "{") {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "该渠道不是订阅凭据渠道"})
		return
	}

	status, err := subscriptionCredentialStatusFromKey(ch.Type, rawKey)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": status})
}

type subscriptionStatusView struct {
	provider       string
	accessToken    string
	refreshToken   string
	expired        string
	lastRefresh    string
	accountID      string
	email          string
}

func keyJSONValue(access, refresh, expired, lastRefresh, accountID, email string) *subscriptionStatusView {
	return &subscriptionStatusView{
		accessToken:  access,
		refreshToken: refresh,
		expired:      expired,
		lastRefresh:  lastRefresh,
		accountID:    accountID,
		email:        email,
	}
}

func subscriptionCredentialStatus(chType int, v *subscriptionStatusView) gin.H {
	return buildSubscriptionStatusView(chType, v)
}

func subscriptionCredentialStatusFromKey(chType int, rawKey string) (gin.H, error) {
	var v subscriptionStatusView
	if chType == constant.ChannelTypeAnthropic {
		cred, err := service.ParseClaudeOAuthKeyForView(rawKey)
		if err != nil {
			return nil, err
		}
		v = subscriptionStatusView{
			accessToken:  cred.AccessToken,
			refreshToken: cred.RefreshToken,
			expired:      cred.Expired,
			lastRefresh:  cred.LastRefresh,
			accountID:    cred.AccountUUID,
			email:        cred.Email,
		}
	} else if chType == constant.ChannelTypeCodex {
		cred, err := service.ParseCodexOAuthKeyForView(rawKey)
		if err != nil {
			return nil, err
		}
		v = subscriptionStatusView{
			accessToken:  cred.AccessToken,
			refreshToken: cred.RefreshToken,
			expired:      cred.Expired,
			lastRefresh:  cred.LastRefresh,
			accountID:    cred.AccountID,
			email:        cred.Email,
		}
	} else if chType == constant.ChannelTypeGemini {
		cred, err := service.ParseGeminiOAuthKeyForView(rawKey)
		if err != nil {
			return nil, err
		}
		v = subscriptionStatusView{
			accessToken:  cred.AccessToken,
			refreshToken: cred.RefreshToken,
			expired:      cred.Expired,
			lastRefresh:  cred.LastRefresh,
			email:        cred.Email,
		}
	} else if chType == constant.ChannelTypeAntigravity {
		cred, err := service.ParseAntigravityOAuthKeyForView(rawKey)
		if err != nil {
			return nil, err
		}
		v = subscriptionStatusView{
			accessToken:  cred.AccessToken,
			refreshToken: cred.RefreshToken,
			expired:      cred.Expired,
			lastRefresh:  cred.LastRefresh,
			email:        cred.Email,
		}
	} else {
		return nil, errors.New("该渠道类型不是订阅凭据渠道")
	}
	return buildSubscriptionStatusView(chType, &v), nil
}

func buildSubscriptionStatusView(chType int, v *subscriptionStatusView) gin.H {
	provider := subscriptionProviderClaude
	switch chType {
	case constant.ChannelTypeCodex:
		provider = subscriptionProviderGPT
	case constant.ChannelTypeGemini:
		provider = subscriptionProviderGemini
	case constant.ChannelTypeAntigravity:
		provider = subscriptionProviderAntigravity
	}

	valid := strings.TrimSpace(v.accessToken) != ""
	remainingSeconds := int64(-1)
	if exp, err := time.Parse(time.RFC3339, strings.TrimSpace(v.expired)); err == nil {
		remainingSeconds = int64(time.Until(exp).Seconds())
	}

	return gin.H{
		"provider":           provider,
		"email":              v.email,
		"account_id":         v.accountID,
		"expired":            v.expired,
		"last_refresh":       v.lastRefresh,
		"remaining_seconds":  remainingSeconds,
		"valid":              valid,
		"auto_refresh":       true,
	}
}
