package store

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// IPRule is a gateway access-control rule for /v1. Kind is 'blacklist'
// (block) or 'whitelist' (allow). CIDR may be a single IP or a network.
// If any enabled whitelist rule exists, only IPs matching a whitelist rule
// are allowed (whitelist-only mode).
type IPRule struct {
	ID        int64
	Kind      string
	CIDR      string
	Note      string
	Enabled   bool
	CreatedAt time.Time
}

const ipRuleColumns = `id, kind, cidr, note, enabled, created_at`

func scanIPRuleRow(row pgx.Row) (*IPRule, error) {
	r := &IPRule{}
	err := row.Scan(&r.ID, &r.Kind, &r.CIDR, &r.Note, &r.Enabled, &r.CreatedAt)
	return r, err
}

func ValidateCIDR(cidr string) error {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return errors.New("empty cidr")
	}
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return fmt.Errorf("invalid CIDR %q", cidr)
	}
	return nil
}

// IPRuleMatches reports whether ip falls inside the stored CIDR rule.
func IPRuleMatches(cidr string, ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if _, network, err := net.ParseCIDR(strings.TrimSpace(cidr)); err == nil {
		return network.Contains(ip)
	}
	return false
}

func (s *Store) ListIPRules(ctx context.Context) ([]IPRule, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+ipRuleColumns+` FROM ip_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]IPRule, 0)
	for rows.Next() {
		r := &IPRule{}
		if err := rows.Scan(&r.ID, &r.Kind, &r.CIDR, &r.Note, &r.Enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *Store) CreateIPRule(ctx context.Context, kind, cidr, note string) (*IPRule, error) {
	cidr = strings.TrimSpace(cidr)
	if err := ValidateCIDR(cidr); err != nil {
		return nil, err
	}
	if kind != "blacklist" && kind != "whitelist" {
		return nil, errors.New("kind must be blacklist or whitelist")
	}
	r := &IPRule{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO ip_rules (kind, cidr, note) VALUES ($1, $2, $3)
		RETURNING `+ipRuleColumns, kind, cidr, note).
		Scan(&r.ID, &r.Kind, &r.CIDR, &r.Note, &r.Enabled, &r.CreatedAt)
	return r, err
}

func (s *Store) UpdateIPRule(ctx context.Context, id int64, note string, enabled bool) (*IPRule, error) {
	r := &IPRule{}
	err := s.pool.QueryRow(ctx, `
		UPDATE ip_rules SET note = $1, enabled = $2 WHERE id = $3
		RETURNING `+ipRuleColumns, note, enabled, id).
		Scan(&r.ID, &r.Kind, &r.CIDR, &r.Note, &r.Enabled, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func (s *Store) DeleteIPRule(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM ip_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
