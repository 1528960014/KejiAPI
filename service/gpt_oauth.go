package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kejiapi/common"
)

// ChatGPT subscription OAuth (authorization-code + PKCE).
// Uses the same public client as the Codex CLI; the token refresh path is
// shared with the Codex credential service.
const (
	gptOAuthAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	gptOAuthRedirectURI  = "https://auth.openai.com/callback"
	gptOAuthScope        = "openid profile email offline_access"
)

type GPTOAuthCredential struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`

	AccountID   string `json:"account_id,omitempty"`
	LastRefresh string `json:"last_refresh,omitempty"`
	Email       string `json:"email,omitempty"`
	Type        string `json:"type,omitempty"`
	Expired     string `json:"expired,omitempty"`
}

type GPTOAuthAuthRequest struct {
	AuthURL      string
	CodeVerifier string
}

// GenerateGPTOAuthAuthURL builds the PKCE authorization URL and returns it
// together with the code verifier (required at exchange time).
func GenerateGPTOAuthAuthURL() (*GPTOAuthAuthRequest, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	challenge := pkces256Challenge(verifier)

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", codexOAuthClientID)
	q.Set("redirect_uri", gptOAuthRedirectURI)
	q.Set("scope", gptOAuthScope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")

	return &GPTOAuthAuthRequest{
		AuthURL:      gptOAuthAuthorizeURL + "?" + q.Encode(),
		CodeVerifier: verifier,
	}, nil
}

func pkces256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ExchangeGPTOAuthCode trades the authorization code (with PKCE verifier)
// for an initial token set and extracts account identity from the id token.
func ExchangeGPTOAuthCode(ctx context.Context, code string, codeVerifier string, proxyURL string) (*GPTOAuthCredential, error) {
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
	form.Set("client_id", codexOAuthClientID)
	form.Set("redirect_uri", gptOAuthRedirectURI)
	form.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexOAuthTokenURL, strings.NewReader(form.Encode()))
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
		IDToken      string `json:"id_token"`
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
		return nil, fmt.Errorf("gpt oauth token request failed: %s", msg)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return nil, errors.New("gpt oauth token response missing access_token")
	}

	cred := &GPTOAuthCredential{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		IDToken:      strings.TrimSpace(payload.IDToken),
		LastRefresh:  time.Now().Format(time.RFC3339),
		Type:         "codex",
	}
	if payload.ExpiresIn > 0 {
		cred.Expired = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	if cred.RefreshToken == "" {
		return nil, errors.New("gpt oauth token response missing refresh_token (subscription login requires offline_access)")
	}
	if accountID, ok := ExtractCodexAccountIDFromJWT(cred.IDToken); ok {
		cred.AccountID = accountID
	}
	if email, ok := ExtractEmailFromJWT(cred.IDToken); ok {
		cred.Email = email
	}
	if cred.AccountID == "" && cred.IDToken == "" {
		return nil, errors.New("gpt oauth token response missing id_token, cannot resolve account")
	}
	return cred, nil
}
