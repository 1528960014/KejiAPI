package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Channel is one upstream endpoint that can serve a model.
type Channel struct {
	ID       int64
	Name     string
	Provider string
	BaseURL  string
	APIKey   string
	ModelID  string
	Priority int
	Enabled  bool
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
