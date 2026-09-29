package store

import (
	"context"
	"fmt"
	"time"
)

// UsageLog records one API call's token usage and cost.
type UsageLog struct {
	ID               int64
	APIKeyID         *int64
	ModelID          string
	Provider         string
	Stream           bool
	PromptTokens     int
	CompletionTokens int
	Cost             float64
	Status           string
	ErrorMsg         string
	CreatedAt        time.Time
}

// LogUsage appends a usage record.
func (s *Store) LogUsage(ctx context.Context, u *UsageLog) error {
	var keyID any
	if u.APIKeyID != nil {
		keyID = *u.APIKeyID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO usage_logs (api_key_id, model_id, provider, stream, prompt_tokens, completion_tokens, cost, status, error_msg)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		keyID, u.ModelID, u.Provider, u.Stream, u.PromptTokens, u.CompletionTokens, u.Cost, u.Status, u.ErrorMsg)
	return err
}

// UsageFilter narrows usage queries; nil APIKeyID means all keys.
type UsageFilter struct {
	APIKeyID *int64
	Limit    int
	Offset   int
}

// ListUsage returns recent usage records, newest first.
func (s *Store) ListUsage(ctx context.Context, f UsageFilter) ([]UsageLog, error) {
	q := `
		SELECT id, api_key_id, model_id, provider, stream, prompt_tokens, completion_tokens, cost, status, error_msg, created_at
		FROM usage_logs`
	args := []any{}
	if f.APIKeyID != nil {
		q += ` WHERE api_key_id = $1`
		args = append(args, *f.APIKeyID)
	}
	q += ` ORDER BY id DESC LIMIT $` + itoa(len(args)+1) + ` OFFSET $` + itoa(len(args)+2)
	args = append(args, f.Limit, f.Offset)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageLog{}
	for rows.Next() {
		var u UsageLog
		if err := rows.Scan(&u.ID, &u.APIKeyID, &u.ModelID, &u.Provider, &u.Stream, &u.PromptTokens, &u.CompletionTokens, &u.Cost, &u.Status, &u.ErrorMsg, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UsageSummary aggregates usage (all-time or since a timestamp).
type UsageSummary struct {
	Requests         int
	PromptTokens     int
	CompletionTokens int
	CostMicro        int64
}

// UsageSummary aggregates usage for a key (nil = all keys), optionally
// restricted to records created since.
func (s *Store) UsageSummary(ctx context.Context, apiKeyID *int64, since *time.Time) (*UsageSummary, error) {
	q := `
		SELECT count(*),
		       COALESCE(sum(prompt_tokens), 0),
		       COALESCE(sum(completion_tokens), 0),
		       CAST(COALESCE(sum(cost), 0) * 1000000 AS BIGINT)
		FROM usage_logs WHERE 1 = 1`
	args := []any{}
	if apiKeyID != nil {
		args = append(args, *apiKeyID)
		q += ` AND api_key_id = $` + itoa(len(args))
	}
	if since != nil {
		args = append(args, *since)
		q += ` AND created_at >= $` + itoa(len(args))
	}
	summary := &UsageSummary{}
	err := s.pool.QueryRow(ctx, q, args...).Scan(&summary.Requests, &summary.PromptTokens, &summary.CompletionTokens, &summary.CostMicro)
	return summary, err
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
