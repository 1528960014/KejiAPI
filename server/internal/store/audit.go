package store

import (
	"context"
	"time"
)

// AuditLog is one recorded mutating admin request. Action is the HTTP
// method + route pattern (e.g. "DELETE /admin/channels/:id"), target holds
// the resolved path params (e.g. "id=42").
type AuditLog struct {
	ID        int64
	Action    string
	Target    string
	Status    int
	IP        string
	CreatedAt time.Time
}

func (s *Store) AddAuditLog(ctx context.Context, l AuditLog) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (action, target, status, ip) VALUES ($1, $2, $3, $4)`,
		l.Action, l.Target, l.Status, l.IP)
	return err
}

// ListAuditLogs returns the most recent audit entries (newest first) plus
// the total row count.
func (s *Store) ListAuditLogs(ctx context.Context, limit, offset int) ([]AuditLog, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, action, target, status, ip, created_at
		FROM audit_logs ORDER BY id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]AuditLog, 0, limit)
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.Action, &l.Target, &l.Status, &l.IP, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}
