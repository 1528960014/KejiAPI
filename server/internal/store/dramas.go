package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Drama is one comic-drama generation: a script fanned out into storyboard
// shots (image + optional TTS per shot). The hold is fixed at submission and
// settled all-or-nothing; the drama UUID is the ledger request ID.
type Drama struct {
	ID              int64
	DramaUUID       string
	APIKeyID        *int64
	KeyUserID       *int64
	KeyOrgID        *int64
	Title           string
	Script          string
	Style           string
	StoryboardModel string
	ImageModel      string
	TTSModel        *string
	ShotsPlanned    int
	Status          string // queued | running | succeeded | failed
	HoldMicro       int64
	Shots           []byte // JSONB: array of shot objects (the asset pack)
	Cost            float64
	ErrorMsg        string
	VideoURL        string
	VideoError      string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

const dramaColumns = `d.id, d.drama_uuid, d.api_key_id, k.user_id, k.org_id, d.title, d.script, d.style, d.storyboard_model, d.image_model, d.tts_model, d.shots_planned, d.status, COALESCE(d.hold_micro, 0), d.shots, COALESCE(d.cost, 0), d.error_msg, COALESCE(d.video_url, ''), COALESCE(d.video_error, ''), d.created_at, d.updated_at`

func dramaSelect() string {
	return `SELECT ` + dramaColumns + ` FROM dramas d LEFT JOIN api_keys k ON k.id = d.api_key_id`
}

func scanDrama(row pgx.Row) (*Drama, error) {
	d := &Drama{}
	err := row.Scan(&d.ID, &d.DramaUUID, &d.APIKeyID, &d.KeyUserID, &d.KeyOrgID, &d.Title, &d.Script, &d.Style, &d.StoryboardModel, &d.ImageModel, &d.TTSModel, &d.ShotsPlanned, &d.Status, &d.HoldMicro, &d.Shots, &d.Cost, &d.ErrorMsg, &d.VideoURL, &d.VideoError, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

func (s *Store) scanDramaRows(rows pgx.Rows) ([]Drama, error) {
	out := []Drama{}
	for rows.Next() {
		var d Drama
		if err := rows.Scan(&d.ID, &d.DramaUUID, &d.APIKeyID, &d.KeyUserID, &d.KeyOrgID, &d.Title, &d.Script, &d.Style, &d.StoryboardModel, &d.ImageModel, &d.TTSModel, &d.ShotsPlanned, &d.Status, &d.HoldMicro, &d.Shots, &d.Cost, &d.ErrorMsg, &d.VideoURL, &d.VideoError, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CreateDrama inserts a queued drama with its frozen hold amount.
func (s *Store) CreateDrama(ctx context.Context, d *Drama) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO dramas (drama_uuid, api_key_id, title, script, style, storyboard_model, image_model, tts_model, shots_planned, hold_micro, shots)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		d.DramaUUID, d.APIKeyID, d.Title, d.Script, d.Style, d.StoryboardModel, d.ImageModel, d.TTSModel, d.ShotsPlanned, d.HoldMicro, d.Shots)
	return err
}

// GetDramaByUUID fetches one drama.
func (s *Store) GetDramaByUUID(ctx context.Context, dramaUUID string) (*Drama, error) {
	return scanDrama(s.pool.QueryRow(ctx, dramaSelect()+` WHERE d.drama_uuid = $1`, dramaUUID))
}

// ClaimNextQueuedDrama atomically claims the oldest queued drama (same
// SKIP LOCKED pattern as media tasks).
func (s *Store) ClaimNextQueuedDrama(ctx context.Context) (*Drama, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE dramas d SET status = 'running', updated_at = now()
		WHERE d.id = (
			SELECT id FROM dramas WHERE status = 'queued' ORDER BY id LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+dramaColumns)
	return scanDrama(row)
}

// UpdateDramaShots persists shot progress (the JSONB asset pack in progress).
func (s *Store) UpdateDramaShots(ctx context.Context, dramaUUID string, shotsJSON []byte) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dramas SET shots = $2, updated_at = now()
		WHERE drama_uuid = $1 AND status = 'running'`, dramaUUID, shotsJSON)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CompleteDrama marks a running drama succeeded with its final cost (USD).
func (s *Store) CompleteDrama(ctx context.Context, dramaUUID string, costUSD float64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dramas SET status = 'succeeded', cost = $2, updated_at = now()
		WHERE drama_uuid = $1 AND status = 'running'`, dramaUUID, costUSD)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FailDrama marks a queued or running drama failed.
func (s *Store) FailDrama(ctx context.Context, dramaUUID, errMsg string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dramas SET status = 'failed', error_msg = $2, updated_at = now()
		WHERE drama_uuid = $1 AND status IN ('queued', 'running')`, dramaUUID, errMsg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDramaVideo records the composed video (or the reason it is missing).
func (s *Store) SetDramaVideo(ctx context.Context, dramaUUID, videoURL, videoErr string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE dramas SET video_url = $2, video_error = $3, updated_at = now()
		WHERE drama_uuid = $1`, dramaUUID, videoURL, videoErr)
	return err
}

// ListRunningDramas returns dramas left running by a dead worker, for startup
// recovery.
func (s *Store) ListRunningDramas(ctx context.Context, limit int) ([]Drama, error) {
	rows, err := s.pool.Query(ctx, dramaSelect()+` WHERE d.status = 'running' ORDER BY d.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanDramaRows(rows)
}

// ListDramas returns the newest dramas, optionally filtered by API key.
func (s *Store) ListDramas(ctx context.Context, apiKeyID *int64, limit int) ([]Drama, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := dramaSelect()
	args := []any{limit}
	if apiKeyID != nil {
		args = append(args, *apiKeyID)
		query += ` WHERE d.api_key_id = $2`
	}
	query += ` ORDER BY d.id DESC LIMIT $1`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanDramaRows(rows)
}
