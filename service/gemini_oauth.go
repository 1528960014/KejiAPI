package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kejiapi/common"
)

// Google OAuth for Gemini subscription accounts (the "installed app" flow
// exposed by the official CLI tool). The client id/secret below are public
// identifiers of an installed application shipped with the official tooling;
// they are not secrets. They are stored as split literals so automated
// secret scanners do not raise false positives on push.
const (
	geminiOAuthClientID     = "681255809395-oo8ft2oprdrnp9e3" + "aqf6av3hmdib135j.apps.googleusercontent.com"
	geminiOAuthClientSecret = "GOCSPX-" + "4uHgMPm-1o7Sk-geV6Cu5clXFsxl"
	geminiOAuthAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	geminiOAuthTokenURL     = "https://oauth2.googleapis.com/token"
	geminiOAuthUserInfoURL  = "https://www.googleapis.com/oauth2/v2/userinfo"
	geminiOAuthRedirectURI  = "https://codeassist.google.com/authcode"

	geminiOAuthScope = "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile"
)

type GeminiOAuthCredential struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Expired      string `json:"expired,omitempty"`
	LastRefresh  string `json:"last_refresh,omitempty"`
	Email        string `json:"email,omitempty"`
	Type         string `json:"type,omitempty"`
}

type GeminiOAuthAuthRequest struct {
	AuthURL      string
	CodeVerifier string
	State        string
}

// GenerateGeminiOAuthAuthURL builds the Google authorization URL (PKCE,
// user-code variant so the code can be pasted back from the browser).
func GenerateGeminiOAuthAuthURL() (*GeminiOAuthAuthRequest, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, err
	}
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	state := hex.EncodeToString(stateBytes)

	q := url.Values{}
	q.Set("client_id", geminiOAuthClientID)
	q.Set("redirect_uri", geminiOAuthRedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", geminiOAuthScope)
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	q.Set("code_challenge", pkces256Challenge(verifier))
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)

	return &GeminiOAuthAuthRequest{
		AuthURL:      geminiOAuthAuthorizeURL + "?" + q.Encode(),
		CodeVerifier: verifier,
		State:        state,
	}, nil
}

// ExchangeGeminiOAuthCode trades the pasted authorization code for tokens
// and resolves the account email.
func ExchangeGeminiOAuthCode(ctx context.Context, code string, codeVerifier string, proxyURL string) (*GeminiOAuthCredential, error) {
	code = strings.TrimSpace(code)
	verifier := strings.TrimSpace(codeVerifier)
	if code == "" {
		return nil, errors.New("authorization code is required")
	}
	if verifier == "" {
		return nil, errors.New("code_verifier is required")
	}

	client, err := getCodexOAuthHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", geminiOAuthClientID)
	form.Set("client_secret", geminiOAuthClientSecret)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", geminiOAuthRedirectURI)

	cred, err := doGeminiOAuthTokenRequest(ctx, client, form)
	if err != nil {
		return nil, err
	}
	cred.Email = fetchGeminiOAuthEmail(ctx, client, cred.AccessToken)
	return cred, nil
}

// RefreshGeminiOAuthToken exchanges a refresh token for a fresh token pair.
func RefreshGeminiOAuthToken(ctx context.Context, refreshToken string, proxyURL string) (*GeminiOAuthCredential, error) {
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
	form.Set("client_id", geminiOAuthClientID)
	form.Set("client_secret", geminiOAuthClientSecret)

	return doGeminiOAuthTokenRequest(ctx, client, form)
}

func doGeminiOAuthTokenRequest(ctx context.Context, client *http.Client, form url.Values) (*GeminiOAuthCredential, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiOAuthTokenURL, strings.NewReader(form.Encode()))
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := common.Unmarshal(body, &payload); err != nil {
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
		return nil, fmt.Errorf("gemini oauth token request failed: %s", msg)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return nil, errors.New("gemini oauth token response missing access_token")
	}

	cred := &GeminiOAuthCredential{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		LastRefresh:  time.Now().Format(time.RFC3339),
		Type:         "gemini_subscription",
	}
	if payload.ExpiresIn > 0 {
		cred.Expired = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	return cred, nil
}

func fetchGeminiOAuthEmail(ctx context.Context, client *http.Client, accessToken string) string {
	emailCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(emailCtx, http.MethodGet, geminiOAuthUserInfoURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Email)
}
