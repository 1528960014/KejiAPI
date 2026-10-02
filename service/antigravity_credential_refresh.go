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

type AntigravityCredentialRefreshOptions struct {
	ResetCaches bool
}

func parseAntigravityOAuthKey(raw string) (*AntigravityCredential, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("antigravity channel: empty oauth key")
	}
	var key AntigravityCredential
	if err := common.Unmarshal([]byte(raw), &key); err != nil {
		return nil, errors.New("antigravity channel: invalid oauth key json")
	}
	return &key, nil
}

// RefreshAntigravityChannelCredential refreshes the Google OAuth token
// stored on a ChannelTypeAntigravity channel whose key is a JSON credential
// object, then writes the updated JSON back to the database. The project id
// and tier discovered at onboarding time are preserved.
func RefreshAntigravityChannelCredential(ctx context.Context, channelID int, opts AntigravityCredentialRefreshOptions) (*AntigravityCredential, *model.Channel, error) {
	ch, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, nil, err
	}
	if ch == nil {
		return nil, nil, fmt.Errorf("channel not found")
	}
	if ch.Type != constant.ChannelTypeAntigravity {
		return nil, nil, fmt.Errorf("channel type is not Antigravity")
	}

	rawKey := strings.TrimSpace(ch.Key)
	if !strings.HasPrefix(rawKey, "{") {
		return nil, nil, errors.New("channel key is not a subscription credential object")
	}
	oauthKey, err := parseAntigravityOAuthKey(rawKey)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(oauthKey.RefreshToken) == "" {
		return nil, nil, errors.New("antigravity channel: refresh_token is required to refresh credential")
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cred, err := RefreshAntigravityOAuthToken(refreshCtx, oauthKey.RefreshToken, ch.GetSetting().Proxy)
	if err != nil {
		return nil, nil, err
	}

	prevRefreshToken := oauthKey.RefreshToken
	oauthKey.AccessToken = cred.AccessToken
	oauthKey.RefreshToken = cred.RefreshToken
	oauthKey.Expired = cred.Expired
	oauthKey.LastRefresh = cred.LastRefresh
	oauthKey.Type = cred.Type
	// Google refresh responses may omit the refresh token; keep the old one.
	if strings.TrimSpace(oauthKey.RefreshToken) == "" {
		oauthKey.RefreshToken = prevRefreshToken
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