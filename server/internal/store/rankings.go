package store

import (
	"context"
	"time"
)

// RankingEntry is one model's totals inside a period window.
type RankingEntry struct {
	ModelID    string
	Provider   string
	Tokens     int64
	Requests   int64
	CostMicro  int64
	PrevTokens int64
}

// RankingBucket is one time-bucket × model cell for the stacked chart.
type RankingBucket struct {
	Bucket  string
	ModelID string
	Tokens  int64
}

// RankingResult is the public rankings payload for one period.
type RankingResult struct {
	Period      string
	TotalTokens int64
	Models      []RankingEntry
	Buckets     []RankingBucket
}

// rankingWindow resolves a period label into (start, prevStart, to-char
// bucket format). All windows are UTC-aligned.
func rankingWindow(period string) (start, prevStart time.Time, bucketFmt string) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch period {
	case "week":
		return today.AddDate(0, 0, -6), today.AddDate(0, 0, -13), "MM-DD"
	case "month":
		return today.AddDate(0, 0, -29), today.AddDate(0, 0, -59), "MM-DD"
	case "year":
		return today.AddDate(-1, 0, 0), today.AddDate(-2, 0, 0), "YYYY-MM"
	default:
		return today, today.AddDate(0, 0, -1), "HH24"
	}
}

// ModelRankings aggregates usage_logs per model for the public rankings
// page: top 12 models with tokens/requests/cost, week-over-period change,
// plus a bucket × model matrix for the stacked bar chart.
func (s *Store) ModelRankings(ctx context.Context, period string) (*RankingResult, error) {
	start, prevStart, bucketFmt := rankingWindow(period)
	end := time.Now().UTC()

	res := &RankingResult{Period: period, Models: []RankingEntry{}, Buckets: []RankingBucket{}}

	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(prompt_tokens + completion_tokens), 0)
		FROM usage_logs WHERE created_at >= $1`, start).Scan(&total); err != nil {
		return nil, err
	}
	res.TotalTokens = total

	rows, err := s.pool.Query(ctx, `
		SELECT model_id,
		       MAX(provider),
		       COALESCE(sum(prompt_tokens + completion_tokens), 0),
		       count(*),
		       CAST(COALESCE(sum(cost), 0) * 1000000 AS BIGINT)
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY model_id
		ORDER BY 3 DESC
		LIMIT 12`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m RankingEntry
		if err := rows.Scan(&m.ModelID, &m.Provider, &m.Tokens, &m.Requests, &m.CostMicro); err != nil {
			return nil, err
		}
		res.Models = append(res.Models, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(res.Models) == 0 {
		return res, nil
	}

	ids := make([]string, 0, len(res.Models))
	for i := range res.Models {
		ids = append(ids, res.Models[i].ModelID)
	}

	prev, err := s.pool.Query(ctx, `
		SELECT model_id, COALESCE(sum(prompt_tokens + completion_tokens), 0)
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY model_id`, prevStart, start)
	if err != nil {
		return nil, err
	}
	defer prev.Close()
	prevMap := map[string]int64{}
	for prev.Next() {
		var id string
		var tokens int64
		if err := prev.Scan(&id, &tokens); err != nil {
			return nil, err
		}
		prevMap[id] = tokens
	}
	if err := prev.Err(); err != nil {
		return nil, err
	}
	for i := range res.Models {
		res.Models[i].PrevTokens = prevMap[res.Models[i].ModelID]
	}

	brows, err := s.pool.Query(ctx, `
		SELECT to_char(created_at AT TIME ZONE 'UTC', $1),
		       model_id,
		       COALESCE(sum(prompt_tokens + completion_tokens), 0)
		FROM usage_logs
		WHERE created_at >= $2 AND created_at < $3 AND model_id = ANY($4)
		GROUP BY 1, 2`, bucketFmt, start, end, ids)
	if err != nil {
		return nil, err
	}
	defer brows.Close()
	for brows.Next() {
		var b RankingBucket
		if err := brows.Scan(&b.Bucket, &b.ModelID, &b.Tokens); err != nil {
			return nil, err
		}
		res.Buckets = append(res.Buckets, b)
	}
	return res, brows.Err()
}