package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"kejiapi/common"
	"kejiapi/constant"
	"kejiapi/model"
)

type ClaudeCredentialRefreshOptions struct {
	ResetCaches bool
}

func parseClaudeOAuthKey(raw string) (*ClaudeOAuthCredential, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("claude channel: empty oauth key")
	}
	var key ClaudeOAuthCredential
	if err := common.Unmarshal([]byte(raw), &key); err != nil {
		return nil, errors.New("claude channel: invalid oauth key json")
	}
	return &key, nil
}

// RefreshClaudeChannelCredential refreshes the subscription OAuth token
// stored on a ChannelTypeAnthropic channel whose key is a JSON credential
// object, then writes the updated JSON back to the database.
func RefreshClaudeChannelCredential(ctx context.Context, channelID int, opts ClaudeCredentialRefreshOptions) (*ClaudeOAuthCredential, *model.Channel, error) {
	ch, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, nil, err
	}
	if ch == nil {
		return nil, nil, fmt.Errorf("channel not found")
	}
	if ch.Type != constant.ChannelTypeAnthropic {
		return nil, nil, fmt.Errorf("channel type is not Anthropic")
	}

	rawKey := strings.TrimSpace(ch.Key)
	if !strings.HasPrefix(rawKey, "{") {
		return nil, nil, errors.New("channel key is not a subscription credential object")
	}
	oauthKey, err := parseClaudeOAuthKey(rawKey)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(oauthKey.RefreshToken) == "" {
		return nil, nil, errors.New("claude channel: refresh_token is required to refresh credential")
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cred, err := RefreshClaudeOAuthToken(refreshCtx, oauthKey.RefreshToken, ch.GetSetting().Proxy)
	if err != nil {
		return nil, nil, err
	}

	oauthKey.AccessToken = cred.AccessToken
	oauthKey.RefreshToken = cred.RefreshToken
	oauthKey.Expired = cred.Expired
	oauthKey.LastRefresh = cred.LastRefresh
	oauthKey.Type = cred.Type
	if strings.TrimSpace(oauthKey.AccountUUID) == "" {
		oauthKey.AccountUUID = cred.AccountUUID
	}
	if strings.TrimSpace(oauthKey.Email) == "" {
		oauthKey.Email = cred.Email
	}

	encoded, err := common.Marshal(oauthKey)
	if err != nil {
		return nil, nil, err
	}
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("key", string(encoded)).Error; err != nil {
		return nil, nil, err
	}

	if opts.ResetCaches {
		model.InitChannelCache()
	}
	return oauthKey, ch, nil
}
