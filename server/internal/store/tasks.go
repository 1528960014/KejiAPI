package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Task is one media generation job (image/video/music/tts). The hold amount
// is fixed at submission; the worker settles it on success or releases it on
// failure, using the task UUID as the ledger request ID.
type Task struct {
	ID         int64
	TaskUUID   string
	APIKeyID   *int64
	KeyUserID  *int64
	Type       string
	ModelID    string
	Payload    []byte
	Status     string // queued | running | succeeded | failed
	HoldMicro  int64
	ResultURLs []string
	Cost       float64
	ErrorMsg   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// taskColumns joins the key's billing user so the worker can settle/release
// without a second lookup.
const taskColumns = `t.id, t.task_uuid, t.api_key_id, k.user_id, t.type, t.model_id, t.payload, t.status, COALESCE(t.hold_micro, 0), t.result_urls, COALESCE(t.cost, 0), t.error_msg, t.created_at, t.updated_at`

func taskSelect() string {
	return `SELECT ` + taskColumns + ` FROM tasks t LEFT JOIN api_keys k ON k.id = t.api_key_id`
}

func scanTask(row pgx.Row) (*Task, error) {
	t := &Task{ResultURLs: []string{}}
	err := row.Scan(&t.ID, &t.TaskUUID, &t.APIKeyID, &t.KeyUserID, &t.Type, &t.ModelID, &t.Payload, &t.Status, &t.HoldMicro, &t.ResultURLs, &t.Cost, &t.ErrorMsg, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// CreateTask inserts a queued task with its hold amount.
func (s *Store) CreateTask(ctx context.Context, t *Task) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tasks (task_uuid, api_key_id, type, model_id, payload, hold_micro)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		t.TaskUUID, t.APIKeyID, t.Type, t.ModelID, t.Payload, t.HoldMicro)
	return err
}

// GetTaskByUUID fetches one task.
func (s *Store) GetTaskByUUID(ctx context.Context, taskUUID string) (*Task, error) {
	row := s.pool.QueryRow(ctx, taskSelect()+` WHERE t.task_uuid = $1`, taskUUID)
	return scanTask(row)
}

// ClaimNextQueued atomically claims the oldest queued task and marks it
// running. Concurrent workers are safe thanks to FOR UPDATE SKIP LOCKED.
func (s *Store) ClaimNextQueued(ctx context.Context) (*Task, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE tasks t SET status = 'running', updated_at = now()
		WHERE t.id = (
			SELECT id FROM tasks WHERE status = 'queued' ORDER BY id LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+taskColumns)
	return scanTask(row)
}

// CompleteTask marks a running task succeeded with its result URLs and final
// cost (USD).
func (s *Store) CompleteTask(ctx context.Context, taskUUID string, urls []string, costUSD float64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks SET status = 'succeeded', result_urls = $2, cost = $3, updated_at = now()
		WHERE task_uuid = $1 AND status = 'running'`, taskUUID, urls, costUSD)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FailTask marks a queued or running task failed.
func (s *Store) FailTask(ctx context.Context, taskUUID, errMsg string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks SET status = 'failed', error_msg = $2, updated_at = now()
		WHERE task_uuid = $1 AND status IN ('queued', 'running')`, taskUUID, errMsg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListRunningTasks returns tasks left running by a dead worker, for startup
// recovery.
func (s *Store) ListRunningTasks(ctx context.Context, limit int) ([]Task, error) {
	rows, err := s.pool.Query(ctx, taskSelect()+` WHERE t.status = 'running' ORDER BY t.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.TaskUUID, &t.APIKeyID, &t.KeyUserID, &t.Type, &t.ModelID, &t.Payload, &t.Status, &t.HoldMicro, &t.ResultURLs, &t.Cost, &t.ErrorMsg, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListTasks returns the newest tasks, optionally filtered by API key.
func (s *Store) ListTasks(ctx context.Context, apiKeyID *int64, limit int) ([]Task, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := taskSelect()
	args := []any{limit}
	if apiKeyID != nil {
		args = append(args, *apiKeyID)
		query += ` WHERE t.api_key_id = $2`
	}
	query += ` ORDER BY t.id DESC LIMIT $1`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.TaskUUID, &t.APIKeyID, &t.KeyUserID, &t.Type, &t.ModelID, &t.Payload, &t.Status, &t.HoldMicro, &t.ResultURLs, &t.Cost, &t.ErrorMsg, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}