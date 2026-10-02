package service

// ParseClaudeOAuthKeyForView is the exported read-only variant of
// parseClaudeOAuthKey, used by the status endpoint to inspect a
// subscription credential without mutating it.
func ParseClaudeOAuthKeyForView(raw string) (*ClaudeOAuthCredential, error) {
	return parseClaudeOAuthKey(raw)
}

// ParseCodexOAuthKeyForView is the exported read-only variant of
// parseCodexOAuthKey, used by the status endpoint to inspect a
// subscription credential without mutating it.
func ParseCodexOAuthKeyForView(raw string) (*CodexOAuthKey, error) {
	return parseCodexOAuthKey(raw)
}

// ParseGeminiOAuthKeyForView is the exported read-only variant of
// parseGeminiOAuthKey, used by the status endpoint to inspect a
// subscription credential without mutating it.
func ParseGeminiOAuthKeyForView(raw string) (*GeminiOAuthCredential, error) {
	return parseGeminiOAuthKey(raw)
}

// ParseAntigravityOAuthKeyForView is the exported read-only variant of
// parseAntigravityOAuthKey, used by the status endpoint to inspect a
// subscription credential without mutating it.
func ParseAntigravityOAuthKeyForView(raw string) (*AntigravityCredential, error) {
	return parseAntigravityOAuthKey(raw)
}
