package controller

import (
	"fmt"
	"net/http"
	"strings"

	"kejiapi/common"
	"kejiapi/constant"
	"kejiapi/model"
	"kejiapi/service"

	"github.com/gin-gonic/gin"
)

type accountPoolImportRequest struct {
	Raw    string `json:"raw"`
	Group  string `json:"group"`
	Models string `json:"models"`
	DryRun bool   `json:"dry_run"`
}

type poolImportProviderMeta struct {
	channelType   int
	label         string
	defaultModels string
}

var poolImportProviders = map[string]poolImportProviderMeta{
	"claude":      {constant.ChannelTypeAnthropic, "Claude", defaultClaudeSubscriptionModels},
	"codex":       {constant.ChannelTypeCodex, "ChatGPT", defaultGPTSubscriptionModels},
	"gemini":      {constant.ChannelTypeGemini, "Gemini", defaultGeminiSubscriptionModels},
	"antigravity": {constant.ChannelTypeAntigravity, "Antigravity", defaultAntigravitySubscriptionModels},
}

// ImportAccountPool bulk-imports subscription accounts from pasted
// credential data. The raw payload may be a JSON array, JSON lines, a
// single object, or concatenated objects; each entry is auto-detected
// (Claude rt-JSON / ChatGPT auth.json / Google exports / native
// credential JSON) and becomes one channel.
//
// POST /api/channel/pool/import
func ImportAccountPool(c *gin.Context) {
	var req accountPoolImportRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	accounts := service.ParseImportedAccounts(req.Raw)
	if len(accounts) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "未解析到可导入账号，请检查粘贴内容"})
		return
	}

	group := strings.TrimSpace(req.Group)
	if group == "" {
		group = "default"
	}

	type importResultItem struct {
		Index     int    `json:"index"`
		Provider  string `json:"provider"`
		Email     string `json:"email"`
		Name      string `json:"name,omitempty"`
		ChannelId int    `json:"channel_id,omitempty"`
		Error     string `json:"error,omitempty"`
	}
	results := make([]importResultItem, 0, len(accounts))

	toCreate := make([]model.Channel, 0, len(accounts))
	indexOf := make([]int, 0, len(accounts))

	nameCount := make(map[string]int)
	for i, acc := range accounts {
		if acc.Err != nil {
			results = append(results, importResultItem{Index: i + 1, Error: acc.Err.Error()})
			continue
		}
		meta, ok := poolImportProviders[acc.Provider]
		if !ok {
			results = append(results, importResultItem{
				Index: i + 1, Provider: acc.Provider, Error: "不支持的提供商类型",
			})
			continue
		}
		var name string
		if acc.Email != "" {
			name = fmt.Sprintf("%s (%s)", meta.label, acc.Email)
		} else {
			name = fmt.Sprintf("%s 导入 #%d", meta.label, i+1)
		}
		nameCount[name]++
		if nameCount[name] > 1 {
			name = fmt.Sprintf("%s-%d", name, nameCount[name])
		}
		models := strings.TrimSpace(req.Models)
		if models == "" {
			models = meta.defaultModels
		}
		if req.DryRun {
			results = append(results, importResultItem{
				Index: i + 1, Provider: acc.Provider, Email: acc.Email, Name: name,
			})
			continue
		}
		weight := uint(10)
		priority := int64(0)
		baseURL := constant.GetChannelBaseURL(meta.channelType)
		ch := model.Channel{
			Type:        meta.channelType,
			Key:         acc.KeyJSON,
			Status:      common.ChannelStatusEnabled,
			Name:        name,
			Weight:      &weight,
			BaseURL:     &baseURL,
			Models:      models,
			Group:       group,
			Priority:    &priority,
			CreatedTime: common.GetTimestamp(),
		}
		toCreate = append(toCreate, ch)
		indexOf = append(indexOf, i)
	}

	created := 0
	if !req.DryRun && len(toCreate) > 0 {
		if err := model.BatchInsertChannels(toCreate); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "渠道创建失败: " + err.Error()})
			return
		}
		model.InitChannelCache()
		for j, ch := range toCreate {
			i := indexOf[j]
			acc := accounts[i]
			results = append(results, importResultItem{
				Index: i + 1, Provider: acc.Provider, Email: acc.Email,
				Name: ch.Name, ChannelId: ch.Id,
			})
			created++
		}
	}

	recordManageAudit(c, "channel.pool_import", map[string]any{
		"total":   len(accounts),
		"created": created,
		"dry_run": req.DryRun,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"total":   len(accounts),
			"created": created,
			"dry_run": req.DryRun,
			"results": results,
		},
	})
}
