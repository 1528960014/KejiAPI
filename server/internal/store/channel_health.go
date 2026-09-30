package store

import (
	"context"
	"time"
)

// ChannelHealthRow is the latest probe snapshot for one channel.
type ChannelHealthRow struct {
	ChannelID int64     `json:"channel_id"`
	OK        bool      `json:"ok"`
	Status    int       `json:"status"`
	LatencyMs int64     `json:"latency_ms"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// UpsertChannelHealth stores the latest probe result for a channel.
func (s *Store) UpsertChannelHealth(ctx context.Context, h *ChannelHealthRow) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO channel_health (channel_id, ok, status, latency_ms, error, checked_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (channel_id) DO UPDATE
		SET ok = EXCLUDED.ok,
		    status = EXCLUDED.status,
		    latency_ms = EXCLUDED.latency_ms,
		    error = EXCLUDED.error,
		    checked_at = EXCLUDED.checked_at`,
		h.ChannelID, h.OK, h.Status, h.LatencyMs, h.Error, h.CheckedAt)
	return err
}

// ListChannelHealth returns the latest probe snapshot for every channel.
func (s *Store) ListChannelHealth(ctx context.Context) (map[int64]*ChannelHealthRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT channel_id, ok, status, latency_ms, error, checked_at
		FROM channel_health`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]*ChannelHealthRow{}
	for rows.Next() {
		var h ChannelHealthRow
		if err := rows.Scan(&h.ChannelID, &h.OK, &h.Status, &h.LatencyMs, &h.Error, &h.CheckedAt); err != nil {
			return nil, err
		}
		cp := h
		out[cp.ChannelID] = &cp
	}
	return out, rows.Err()
}
