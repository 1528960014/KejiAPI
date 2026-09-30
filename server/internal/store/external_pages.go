package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// P7-6: external systems. An admin can register external web pages
// (ticketing, monitoring, docs, ...) that the admin console renders as
// full-height iframes in the sidebar, extending the management panel with
// third-party integrations without a custom deployment.
type ExternalPage struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Enabled   bool      `json:"enabled"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

const externalPageColumns = `id, name, url, enabled, sort_order, created_at`

func scanExternalPage(row pgx.Row) (*ExternalPage, error) {
	p := &ExternalPage{}
	err := row.Scan(&p.ID, &p.Name, &p.URL, &p.Enabled, &p.SortOrder, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func scanExternalPageRows(rows pgx.Rows) ([]ExternalPage, error) {
	out := []ExternalPage{}
	for rows.Next() {
		var p ExternalPage
		if err := rows.Scan(&p.ID, &p.Name, &p.URL, &p.Enabled, &p.SortOrder, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListExternalPages returns all registered pages in display order.
func (s *Store) ListExternalPages(ctx context.Context) ([]ExternalPage, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+externalPageColumns+` FROM external_pages ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExternalPageRows(rows)
}

// CreateExternalPage registers a new external page.
func (s *Store) CreateExternalPage(ctx context.Context, name, url string, sortOrder int) (*ExternalPage, error) {
	if name == "" || url == "" {
		return nil, errors.New("name and url are required")
	}
	created := &ExternalPage{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO external_pages (name, url, sort_order)
		VALUES ($1, $2, $3)
		RETURNING `+externalPageColumns, name, url, sortOrder,
	).Scan(&created.ID, &created.Name, &created.URL, &created.Enabled, &created.SortOrder, &created.CreatedAt)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// ExternalPagePatch carries the mutable fields; nil fields are unchanged.
type ExternalPagePatch struct {
	Name      *string
	URL       *string
	Enabled   *bool
	SortOrder *int
}

// UpdateExternalPage applies a partial update and returns the fresh page.
func (s *Store) UpdateExternalPage(ctx context.Context, id int64, p *ExternalPagePatch) (*ExternalPage, error) {
	sets := []string{}
	args := []any{}
	nextArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if p.Name != nil {
		sets = append(sets, "name="+nextArg(*p.Name))
	}
	if p.URL != nil {
		sets = append(sets, "url="+nextArg(*p.URL))
	}
	if p.Enabled != nil {
		sets = append(sets, "enabled="+nextArg(*p.Enabled))
	}
	if p.SortOrder != nil {
		sets = append(sets, "sort_order="+nextArg(*p.SortOrder))
	}
	if len(sets) == 0 {
		return scanExternalPage(s.pool.QueryRow(ctx,
			`SELECT `+externalPageColumns+` FROM external_pages WHERE id = $1`, id))
	}
	args = append(args, id)
	return scanExternalPage(s.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE external_pages SET %s WHERE id = $%d
		RETURNING `+externalPageColumns, strings.Join(sets, ", "), len(args)+1), args...))
}

// DeleteExternalPage removes a registered page.
func (s *Store) DeleteExternalPage(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM external_pages WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
