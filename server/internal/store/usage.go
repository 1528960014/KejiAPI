package store

import (
	"context"
)

// UsageLog records one API call's token usage and cost.
type UsageLog struct {
	APIKeyID          int64
	ModelID           string
	Provider          string
	Stream            bool
	PromptTokens      int
	CompletionTokens  int
	Cost              float64
	Status            string
	ErrorMsg          string
}

// LogUsage appends a usage record.
func (s *Store) LogUsage(ctx context.Context, u *UsageLog) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO usage_logs (api_key_id, model_id, provider, stream, prompt_tokens, completion_tokens, cost, status, error_msg)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		u.APIKeyID, u.ModelID, u.Provider, u.Stream, u.PromptTokens, u.CompletionTokens, u.Cost, u.Status, u.ErrorMsg)
	return err
}