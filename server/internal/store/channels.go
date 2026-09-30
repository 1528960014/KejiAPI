package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Channel is one upstream endpoint that can serve a model.
type Channel struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	ModelID  string `json:"model_id"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

// PickChannel returns the highest-priority enabled channel for a model.
func (s *Store) PickChannel(ctx context.Context, modelID string) (*Channel, error) {
	c := &Channel{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, provider, base_url, api_key, priority
		FROM channels
		WHERE model_id = $1 AND enabled
		ORDER BY priority DESC, id ASC
		LIMIT 1`, modelID,
	).Scan(&c.ID, &c.Name, &c.Provider, &c.BaseURL, &c.APIKey, &c.Priority)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ChannelsForModel returns all enabled channels for a model in failover
// order (priority DESC, id ASC). P5-1: the gateway dials them in sequence
// when a channel fails.
func (s *Store) ChannelsForModel(ctx context.Context, modelID string) ([]*Channel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, provider, base_url, api_key, priority
		FROM channels
		WHERE model_id = $1 AND enabled
		ORDER BY priority DESC, id ASC`, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Channel{}
	for rows.Next() {
		c := &Channel{}
		if err := rows.Scan(&c.ID, &c.Name, &c.Provider, &c.BaseURL, &c.APIKey, &c.Priority); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListChannels returns all channels.
func (s *Store) ListChannels(ctx context.Context) ([]Channel, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, provider, base_url, api_key, model_id, priority, enabled FROM channels ORDER BY model_id, priority DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Channel{}
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Provider, &c.BaseURL, &c.APIKey, &c.ModelID, &c.Priority, &c.Enabled); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateChannel inserts a channel.
func (s *Store) CreateChannel(ctx context.Context, c *Channel) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO channels (name, provider, base_url, api_key, model_id, priority, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		c.Name, c.Provider, c.BaseURL, c.APIKey, c.ModelID, c.Priority, c.Enabled)
	return err
}

// GetChannel fetches one channel by ID.
func (s *Store) GetChannel(ctx context.Context, id int64) (*Channel, error) {
	c := &Channel{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, provider, base_url, api_key, model_id, priority, enabled
		FROM channels WHERE id = $1`, id,
	).Scan(&c.ID, &c.Name, &c.Provider, &c.BaseURL, &c.APIKey, &c.ModelID, &c.Priority, &c.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// ChannelPatch is a partial channel update; nil fields are left untouched.
type ChannelPatch struct {
	Name     *string
	Provider *string
	BaseURL  *string
	APIKey   *string
	ModelID  *string
	Priority *int
	Enabled  *bool
}

// UpdateChannel partially updates a channel and returns the fresh row.
func (s *Store) UpdateChannel(ctx context.Context, id int64, p *ChannelPatch) (*Channel, error) {
	sets := []string{}
	args := []any{}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, col+" = $"+itoa(len(args)))
	}
	if p.Name != nil {
		add("name", *p.Name)
	}
	if p.Provider != nil {
		add("provider", *p.Provider)
	}
	if p.BaseURL != nil {
		add("base_url", *p.BaseURL)
	}
	if p.APIKey != nil {
		add("api_key", *p.APIKey)
	}
	if p.ModelID != nil {
		add("model_id", *p.ModelID)
	}
	if p.Priority != nil {
		add("priority", *p.Priority)
	}
	if p.Enabled != nil {
		add("enabled", *p.Enabled)
	}
	if len(sets) == 0 {
		return s.GetChannel(ctx, id)
	}
	args = append(args, id)
	tag, err := s.pool.Exec(ctx, `UPDATE channels SET `+strings.Join(sets, ", ")+` WHERE id = $`+itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetChannel(ctx, id)
}

// DeleteChannel removes a channel by ID.
func (s *Store) DeleteChannel(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM channels WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// EnsureSeedModel inserts a demo model when the table is empty so first-run
// developers can smoke-test the gateway immediately.
func (s *Store) EnsureSeedModel(ctx context.Context) error {
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM models`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO models (model_id, provider, upstream_model, capabilities, input_price_per_1k, output_price_per_1k)
		VALUES ('demo-echo', 'demo', 'demo-echo', ARRAY['chat'], 0, 0)`,
	)
	return err
}
