package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// Model describes an upstream-facing model and its pricing.
type Model struct {
	ModelID          string
	Provider         string
	UpstreamModel    string
	Capabilities     []string
	InputPricePer1k  float64
	OutputPricePer1k float64
	Enabled          bool
}

const modelColumns = `model_id, provider, upstream_model, capabilities, COALESCE(input_price_per_1k, 0), COALESCE(output_price_per_1k, 0), enabled`

func scanModel(row pgx.Row) (*Model, error) {
	m := &Model{}
	err := row.Scan(&m.ModelID, &m.Provider, &m.UpstreamModel, &m.Capabilities, &m.InputPricePer1k, &m.OutputPricePer1k, &m.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// ListModels returns all models.
func (s *Store) ListModels(ctx context.Context) ([]Model, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+modelColumns+` FROM models ORDER BY model_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Model{}
	for rows.Next() {
		var m Model
		if err := rows.Scan(&m.ModelID, &m.Provider, &m.UpstreamModel, &m.Capabilities, &m.InputPricePer1k, &m.OutputPricePer1k, &m.Enabled); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetModel fetches one model by its public ID.
func (s *Store) GetModel(ctx context.Context, modelID string) (*Model, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+modelColumns+` FROM models WHERE model_id = $1`, modelID)
	return scanModel(row)
}

// CreateModel inserts or updates a model definition.
func (s *Store) CreateModel(ctx context.Context, m *Model) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO models (model_id, provider, upstream_model, capabilities, input_price_per_1k, output_price_per_1k, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (model_id) DO UPDATE SET
			provider = EXCLUDED.provider,
			upstream_model = EXCLUDED.upstream_model,
			capabilities = EXCLUDED.capabilities,
			input_price_per_1k = EXCLUDED.input_price_per_1k,
			output_price_per_1k = EXCLUDED.output_price_per_1k,
			enabled = EXCLUDED.enabled`,
		m.ModelID, m.Provider, m.UpstreamModel, m.Capabilities, m.InputPricePer1k, m.OutputPricePer1k, m.Enabled)
	return err
}

// DeleteModel removes a model (and cascades its channels).
func (s *Store) DeleteModel(ctx context.Context, modelID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM models WHERE model_id = $1`, modelID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
