package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Assistant is a predefined chat-agent template: a system prompt bound to a
// real chat model. Clients invoke it by sending model = <agent_id> to
// /v1/chat/completions; the gateway injects the system prompt and routes to
// ModelID.
type Assistant struct {
	ID           int64
	AgentID      string
	Name         string
	Description  string
	SystemPrompt string
	ModelID      string
	Enabled      bool
	CreatedAt    time.Time
}

const assistantColumns = `id, agent_id, name, description, system_prompt, model_id, enabled, created_at`

func scanAssistant(row pgx.Row) (*Assistant, error) {
	a := &Assistant{}
	err := row.Scan(&a.ID, &a.AgentID, &a.Name, &a.Description, &a.SystemPrompt, &a.ModelID, &a.Enabled, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *Store) modelExists(ctx context.Context, modelID string) (bool, error) {
	var ok int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM models WHERE model_id = $1`, modelID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return true, err
}

// GetAssistantByID resolves the public agent id (not the numeric PK).
func (s *Store) GetAssistantByID(ctx context.Context, agentID string) (*Assistant, error) {
	return scanAssistant(s.pool.QueryRow(ctx, `SELECT `+assistantColumns+` FROM assistants WHERE agent_id = $1`, agentID))
}

// ListAssistants returns all templates (admin view), oldest first.
func (s *Store) ListAssistants(ctx context.Context) ([]Assistant, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+assistantColumns+` FROM assistants ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAssistantRows(rows)
}

// ListEnabledAssistants returns the public template list, oldest first.
func (s *Store) ListEnabledAssistants(ctx context.Context) ([]Assistant, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+assistantColumns+` FROM assistants WHERE enabled ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAssistantRows(rows)
}

func scanAssistantRows(rows pgx.Rows) ([]Assistant, error) {
	out := []Assistant{}
	for rows.Next() {
		var a Assistant
		if err := rows.Scan(&a.ID, &a.AgentID, &a.Name, &a.Description, &a.SystemPrompt, &a.ModelID, &a.Enabled, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateAssistant inserts a template. The bound model must exist.
func (s *Store) CreateAssistant(ctx context.Context, a *Assistant) (*Assistant, error) {
	if a.AgentID == "" || a.SystemPrompt == "" || a.ModelID == "" {
		return nil, errors.New("agent_id, system_prompt and model_id are required")
	}
	ok, err := s.modelExists(ctx, a.ModelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("model %q not found", a.ModelID)
	}
	created := &Assistant{}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO assistants (agent_id, name, description, system_prompt, model_id, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+assistantColumns,
		a.AgentID, a.Name, a.Description, a.SystemPrompt, a.ModelID, a.Enabled,
	).Scan(&created.ID, &created.AgentID, &created.Name, &created.Description, &created.SystemPrompt, &created.ModelID, &created.Enabled, &created.CreatedAt)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// AssistantPatch carries the mutable fields for UpdateAssistant; nil fields
// are left unchanged.
type AssistantPatch struct {
	Name         *string
	Description  *string
	SystemPrompt *string
	ModelID      *string
	Enabled      *bool
}

// UpdateAssistant applies a partial update and returns the fresh template.
func (s *Store) UpdateAssistant(ctx context.Context, id int64, p *AssistantPatch) (*Assistant, error) {
	sets := []string{}
	args := []any{}
	nextArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if p.Name != nil {
		sets = append(sets, "name="+nextArg(*p.Name))
	}
	if p.Description != nil {
		sets = append(sets, "description="+nextArg(*p.Description))
	}
	if p.SystemPrompt != nil {
		sets = append(sets, "system_prompt="+nextArg(*p.SystemPrompt))
	}
	if p.ModelID != nil {
		if *p.ModelID != "" {
			ok, err := s.modelExists(ctx, *p.ModelID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("model %q not found", *p.ModelID)
			}
		}
		sets = append(sets, "model_id="+nextArg(*p.ModelID))
	}
	if p.Enabled != nil {
		sets = append(sets, "enabled="+nextArg(*p.Enabled))
	}
	if len(sets) == 0 {
		return scanAssistant(s.pool.QueryRow(ctx, `SELECT `+assistantColumns+` FROM assistants WHERE id = $1`, id))
	}
	args = append(args, id)
	return scanAssistant(s.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE assistants SET %s WHERE id = $%d
		RETURNING `+assistantColumns, strings.Join(sets, ", "), len(args)+1), args...))
}

// DeleteAssistant removes a template by numeric ID.
func (s *Store) DeleteAssistant(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM assistants WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
