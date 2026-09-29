package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PaySettings holds the platform-wide recharge rate and public base URL.
// CNYPerUSD == 0 disables online recharge.
type PaySettings struct {
	CNYPerUSD float64
	PublicURL string
}

// PayChannel is one payment-channel row: enabled flag plus the opaque
// credential JSON (field names are defined by the api layer).
type PayChannel struct {
	ChannelID string
	Enabled   bool
	Config    json.RawMessage
	UpdatedAt time.Time
}

// GetPaySettings returns the settings row, or zero values when unset.
func (s *Store) GetPaySettings(ctx context.Context) (PaySettings, error) {
	out := PaySettings{}
	err := s.pool.QueryRow(ctx, `SELECT cny_per_usd, public_url FROM pay_settings WHERE id = 1`).
		Scan(&out.CNYPerUSD, &out.PublicURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	return out, err
}

// PaySettingsExists reports whether settings have been saved (by the admin
// or by the first-start env seed).
func (s *Store) PaySettingsExists(ctx context.Context) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pay_settings WHERE id = 1)`).Scan(&ok)
	return ok, err
}

// SetPaySettings upserts the settings row.
func (s *Store) SetPaySettings(ctx context.Context, in PaySettings) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pay_settings (id, cny_per_usd, public_url, updated_at)
		VALUES (1, $1, $2, now())
		ON CONFLICT (id) DO UPDATE
		SET cny_per_usd = EXCLUDED.cny_per_usd,
		    public_url = EXCLUDED.public_url,
		    updated_at = now()`, in.CNYPerUSD, in.PublicURL)
	return err
}

// ListPayChannels returns the stored channel rows (possibly empty).
func (s *Store) ListPayChannels(ctx context.Context) ([]PayChannel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT channel_id, enabled, config, updated_at FROM pay_channels ORDER BY channel_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PayChannel{}
	for rows.Next() {
		var c PayChannel
		if err := rows.Scan(&c.ChannelID, &c.Enabled, &c.Config, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetPayChannel upserts one channel row.
func (s *Store) SetPayChannel(ctx context.Context, c *PayChannel) error {
	cfg := c.Config
	if len(cfg) == 0 {
		cfg = []byte("{}")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pay_channels (channel_id, enabled, config, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (channel_id) DO UPDATE
		SET enabled = EXCLUDED.enabled,
		    config = EXCLUDED.config,
		    updated_at = now()`, c.ChannelID, c.Enabled, cfg)
	return err
}