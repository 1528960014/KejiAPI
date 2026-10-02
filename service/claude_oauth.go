package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kejiapi/common"
)

// Claude subscription OAuth (authorization-code grant).
// Endpoints and the public client id are the ones exposed by the
// official CLI tool; the flow is a standard RFC 6749 auth-code exchange.
const (
	claudeOAuthClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	claudeOAuthAuthorizeURL = "https://console.anthropic.com/v1/oauth/authorize"
	claudeOAuthTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	claudeOAuthRedirectURI  = "http://localhost:54545/oauth/callback"
	claudeOAuthScope        = "user:inference"
)

type ClaudeOAuthCredential struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Expired      string `json:"expired,omitempty"`
	LastRefresh  string `json:"last_refresh,omitempty"`
	AccountUUID  string `json:"account_uuid,omitempty"`
	Email        string `json:"email,omitempty"`
	Type         string `json:"type,omitempty"`
}

// GenerateClaudeOAuthAuthURL builds the authorization URL the admin must
// open in a browser. It returns the URL and the random state value.
func GenerateClaudeOAuthAuthURL() (string, string, error) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", err
	}
	state := hex.EncodeToString(stateBytes)

	q := url.Values{}
	q.Set("code", "true")
	q.Set("client_id", claudeOAuthClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", claudeOAuthRedirectURI)
	q.Set("scope", claudeOAuthScope)
	q.Set("state", state)

	return claudeOAuthAuthorizeURL + "?" + q.Encode(), state, nil
}

// ExchangeClaudeOAuthCode trades the one-time authorization code for an
// initial access/refresh token pair.
func ExchangeClaudeOAuthCode(ctx context.Context, code string, proxyURL string) (*ClaudeOAuthCredential, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("authorization code is required")
	}
	client, err := getCodexOAuthHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", claudeOAuthClientID)
	form.Set("redirect_uri", claudeOAuthRedirectURI)

	return doClaudeOAuthTokenRequest(ctx, client, form)
}

// RefreshClaudeOAuthToken exchanges a refresh token for a fresh token pair.
func RefreshClaudeOAuthToken(ctx context.Context, refreshToken string, proxyURL string) (*ClaudeOAuthCredential, error) {
	rt := strings.TrimSpace(refreshToken)
	if rt == "" {
		return nil, errors.New("refresh_token is required")
	}
	client, err := getCodexOAuthHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", rt)
	form.Set("client_id", claudeOAuthClientID)

	return doClaudeOAuthTokenRequest(ctx, client, form)
}

func doClaudeOAuthTokenRequest(ctx context.Context, client *http.Client, form url.Values) (*ClaudeOAuthCredential, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(payload.ErrorDesc)
		if msg == "" {
			msg = strings.TrimSpace(payload.Error)
		}
		if msg == "" {
			msg = fmt.Sprintf("status=%d", resp.StatusCode)
		}
		return nil, fmt.Errorf("claude oauth token request failed: %s", msg)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return nil, errors.New("claude oauth token response missing access_token")
	}

	cred := &ClaudeOAuthCredential{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		LastRefresh:  time.Now().Format(time.RFC3339),
		Type:         "claude_subscription",
	}
	if payload.ExpiresIn > 0 {
		cred.Expired = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	if uuid, email, ok := extractClaudeAccountFromJWT(cred.AccessToken); ok {
		cred.AccountUUID = uuid
		cred.Email = email
	}
	return cred, nil
}

// extractClaudeAccountFromJWT reads the anthropic-scoped claims embedded in
// the access token (uuid and email of the subscription account).
func extractClaudeAccountFromJWT(token string) (uuid string, email string, ok bool) {
	claims, ok := decodeJWTClaims(token)
	if !ok {
		return "", "", false
	}
	raw, exists := claims["https://api.anthropic.com"]
	if !exists {
		return "", "", false
	}
	obj, isObj := raw.(map[string]any)
	if !isObj {
		return "", "", false
	}
	if v, ok := obj["uuid"].(string); ok {
		uuid = strings.TrimSpace(v)
	}
	if v, ok := obj["email"].(string); ok {
		email = strings.TrimSpace(v)
	}
	if uuid == "" && email == "" {
		return "", "", false
	}
	return uuid, email, true
}
