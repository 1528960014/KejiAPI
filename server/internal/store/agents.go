package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrInvalidRate is returned when an agent rate is outside (0, 1].
var ErrInvalidRate = errors.New("invalid agent rate")

// Agent is a reseller: a billing user whose balance settles every hold at
// the wholesale Rate (0 < rate <= 1) instead of retail.
type Agent struct {
	ID        int64
	UserID    int64
	Rate      float64
	CreatedAt time.Time
}

// AgentRow is an agent joined with its user's email, for admin listings.
type AgentRow struct {
	Agent
	Email string
}

const agentColumns = `id, user_id, rate, created_at`

func scanAgent(row pgx.Row) (*Agent, error) {
	a := &Agent{}
	err := row.Scan(&a.ID, &a.UserID, &a.Rate, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func validateRate(rate float64) error {
	if rate <= 0 || rate > 1 {
		return ErrInvalidRate
	}
	return nil
}

// CreateAgent marks an existing billing user as a reseller at the given
// wholesale rate. Fails with a duplicate-key error if the user is already
// an agent.
func (s *Store) CreateAgent(ctx context.Context, userID int64, rate float64) (*Agent, error) {
	if err := validateRate(rate); err != nil {
		return nil, err
	}
	var ok int
	if err := s.pool.QueryRow(ctx, `SELECT 1 FROM users WHERE id = $1`, userID).Scan(&ok); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	a := &Agent{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO agents (user_id, rate) VALUES ($1, $2)
		RETURNING `+agentColumns, userID, rate,
	).Scan(&a.ID, &a.UserID, &a.Rate, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// GetAgent fetches one agent by ID.
func (s *Store) GetAgent(ctx context.Context, id int64) (*Agent, error) {
	return scanAgent(s.pool.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = $1`, id))
}

// GetAgentByUserID fetches the agent record of a billing user, if any.
func (s *Store) GetAgentByUserID(ctx context.Context, userID int64) (*Agent, error) {
	return scanAgent(s.pool.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE user_id = $1`, userID))
}

// ListAgents returns all agents joined with their user email, oldest first.
func (s *Store) ListAgents(ctx context.Context) ([]AgentRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.user_id, a.rate, a.created_at, u.email
		FROM agents a JOIN users u ON u.id = a.user_id
		ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentRow{}
	for rows.Next() {
		var r AgentRow
		if err := rows.Scan(&r.ID, &r.UserID, &r.Rate, &r.CreatedAt, &r.Email); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateAgentRate changes an agent's wholesale rate.
func (s *Store) UpdateAgentRate(ctx context.Context, id int64, rate float64) (*Agent, error) {
	if err := validateRate(rate); err != nil {
		return nil, err
	}
	return scanAgent(s.pool.QueryRow(ctx, `
		UPDATE agents SET rate = $2 WHERE id = $1
		RETURNING `+agentColumns, id, rate))
}

// DeleteAgent removes an agency. Its subkeys survive as regular keys bound
// to the agent's user (agent_id is set to NULL by the foreign key).
func (s *Store) DeleteAgent(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
