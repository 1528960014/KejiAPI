package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"kejiapi/common"
)

// ImportedAccount is one normalized subscription account parsed from
// pasted import data. Supported input shapes:
//   - a JSON array of credential objects
//   - JSON lines (one credential object per line)
//   - a single credential object
//   - multiple concatenated credential objects
//
// Supported credential dialects are auto-detected:
//   - native credential JSON produced by this system (snake_case)
//   - Claude rt-JSON exports (camelCase: refreshToken / accessToken /
//     expiresAt / account_uuid)
//   - ChatGPT/Codex auth.json (nested "tokens" object or flat fields,
//     email recovered from the id_token JWT when absent)
//   - Google/Gemini exports (accessToken / refreshToken or snake_case)
//   - explicit "provider"/"type"/"platform" field overrides detection
type ImportedAccount struct {
	// Provider is one of: claude, codex, gemini, antigravity
	Provider string
	Email    string
	// KeyJSON is the normalized native credential JSON ready to be
	// stored as the channel key.
	KeyJSON string
	// Err is set when this entry could not be parsed.
	Err error
}

// ParseImportedAccounts parses raw pasted text into normalized accounts.
// Returns nil when the input is empty.
func ParseImportedAccounts(raw string) []ImportedAccount {
	entries := splitImportEntries(raw)
	if len(entries) == 0 {
		return nil
	}
	out := make([]ImportedAccount, 0, len(entries))
	for i, entry := range entries {
		acc := normalizeImportEntry(entry)
		if acc.Err != nil {
			acc.Err = fmt.Errorf("第 %d 条：%v", i+1, acc.Err)
		}
		out = append(out, acc)
	}
	return out
}

// splitImportEntries breaks the raw text into individual JSON objects.
func splitImportEntries(raw string) []map[string]any {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil
	}

	// JSON array of objects.
	if strings.HasPrefix(text, "[") {
		var arr []map[string]any
		if err := json.Unmarshal([]byte(text), &arr); err == nil {
			return arr
		}
	}

	// One or more whitespace-separated JSON objects (covers a single
	// object and concatenated objects; JSON lines also work because
	// newlines are whitespace).
	var entries []map[string]any
	var decErr error
	dec := json.NewDecoder(strings.NewReader(text))
	for {
		var obj map[string]any
		err := dec.Decode(&obj)
		if err != nil {
			decErr = err
			break
		}
		entries = append(entries, obj)
	}
	if decErr == nil {
		return entries
	}

	// Fallback: JSON lines.
	var lines []map[string]any
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		lines = append(lines, obj)
	}
	return lines
}

func importStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// normalizeImportEntry converts one raw credential object into the native
// credential format for the detected provider.
func normalizeImportEntry(m map[string]any) ImportedAccount {
	if m == nil {
		return ImportedAccount{Err: errors.New("空条目")}
	}

	provider := strings.ToLower(importStr(m, "provider", "type_hint", "platform"))
	nestedTokens, _ := m["tokens"].(map[string]any)

	switch {
	case provider == "claude" || provider == "anthropic" ||
		importStr(m, "account_uuid", "accountUUID") != "" ||
		strings.HasPrefix(importStr(m, "refreshToken", "refresh_token"), "sec_"):
		provider = "claude"
	case provider == "chatgpt" || provider == "codex" || provider == "gpt" || provider == "openai" ||
		importStr(m, "id_token", "idToken") != "" ||
		(nestedTokens != nil && importStr(nestedTokens, "id_token", "refresh_token") != "") ||
		importStr(m, "OPENAI_API_KEY") != "":
		provider = "codex"
	case provider == "antigravity" || provider == "agy" || importStr(m, "project_id", "projectId") != "":
		provider = "antigravity"
	case provider == "gemini" || provider == "google":
		provider = "gemini"
	case strings.Contains(strings.ToLower(importStr(m, "email", "email_address")), "@gmail.com"):
		provider = "gemini"
	default:
		// Claude rt-JSON is the most common pasted dialect.
		provider = "claude"
	}

	switch provider {
	case "claude":
		cred := ClaudeOAuthCredential{
			AccessToken:  importStr(m, "access_token", "accessToken"),
			RefreshToken: importStr(m, "refresh_token", "refreshToken"),
			Expired:      importStr(m, "expired", "expiresAt", "expires_at"),
			LastRefresh:  importStr(m, "last_refresh", "lastRefresh"),
			AccountUUID:  importStr(m, "account_uuid", "accountUUID"),
			Email:        importStr(m, "email", "email_address"),
			Type:         "claude_subscription",
		}
		if cred.RefreshToken == "" && cred.AccessToken == "" {
			return ImportedAccount{Provider: provider, Email: cred.Email, Err: errors.New("缺少 refresh_token / access_token")}
		}
		return marshalImportedAccount(provider, cred.Email, cred)

	case "codex":
		access := importStr(m, "access_token", "accessToken")
		idToken := importStr(m, "id_token", "idToken")
		refresh := importStr(m, "refresh_token", "refreshToken")
		expires := importStr(m, "expired", "expires_at", "expiresAt")
		if nestedTokens != nil {
			if access == "" {
				access = importStr(nestedTokens, "access_token", "accessToken")
			}
			if idToken == "" {
				idToken = importStr(nestedTokens, "id_token", "idToken")
			}
			if refresh == "" {
				refresh = importStr(nestedTokens, "refresh_token", "refreshToken")
			}
			if expires == "" {
				expires = importStr(nestedTokens, "expires_at", "expiresAt")
			}
		}
		email := importStr(m, "email", "email_address")
		if email == "" {
			email = jwtEmail(idToken)
		}
		cred := CodexOAuthKey{
			IDToken:      idToken,
			AccessToken:  access,
			RefreshToken: refresh,
			AccountID:    importStr(m, "account_id", "accountId"),
			LastRefresh:  importStr(m, "last_refresh", "lastRefresh"),
			Email:        email,
			Type:         "codex",
			Expired:      expires,
		}
		if cred.RefreshToken == "" && cred.AccessToken == "" {
			return ImportedAccount{Provider: provider, Email: email, Err: errors.New("缺少 refresh_token / access_token")}
		}
		return marshalImportedAccount(provider, email, cred)

	case "gemini":
		cred := GeminiOAuthCredential{
			AccessToken:  importStr(m, "access_token", "accessToken"),
			RefreshToken: importStr(m, "refresh_token", "refreshToken"),
			Expired:      importStr(m, "expired", "expiresAt", "expires_at"),
			LastRefresh:  importStr(m, "last_refresh", "lastRefresh"),
			Email:        importStr(m, "email", "email_address"),
			Type:         "gemini_subscription",
		}
		if cred.RefreshToken == "" && cred.AccessToken == "" {
			return ImportedAccount{Provider: provider, Email: cred.Email, Err: errors.New("缺少 refresh_token / access_token")}
		}
		return marshalImportedAccount(provider, cred.Email, cred)

	case "antigravity":
		cred := AntigravityCredential{
			AccessToken:  importStr(m, "access_token", "accessToken"),
			RefreshToken: importStr(m, "refresh_token", "refreshToken"),
			Expired:      importStr(m, "expired", "expiresAt", "expires_at"),
			LastRefresh:  importStr(m, "last_refresh", "lastRefresh"),
			Email:        importStr(m, "email", "email_address"),
			ProjectID:    importStr(m, "project_id", "projectId"),
			Tier:         importStr(m, "tier"),
			Type:         "antigravity_subscription",
		}
		if cred.RefreshToken == "" && cred.AccessToken == "" {
			return ImportedAccount{Provider: provider, Email: cred.Email, Err: errors.New("缺少 refresh_token / access_token")}
		}
		return marshalImportedAccount(provider, cred.Email, cred)

	default:
		return ImportedAccount{Err: fmt.Errorf("无法识别的凭据格式（provider=%s）", provider)}
	}
}

func marshalImportedAccount(provider, email string, cred any) ImportedAccount {
	b, err := common.Marshal(cred)
	if err != nil {
		return ImportedAccount{Provider: provider, Email: email, Err: err}
	}
	return ImportedAccount{Provider: provider, Email: email, KeyJSON: string(b)}
}

// jwtEmail extracts the email claim from a JWT payload without
// verifying the signature (import-time metadata only).
func jwtEmail(jwt string) string {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return ""
	}
	payload := parts[1]
	// Tolerate missing base64url padding.
	if rem := len(payload) % 4; rem != 0 {
		payload += strings.Repeat("=", 4-rem)
	}
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return ""
	}
	if v, ok := claims["email"].(string); ok {
		return v
	}
	if v, ok := claims["email_address"].(string); ok {
		return v
	}
	return ""
}
