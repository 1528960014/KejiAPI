package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// P3-3: multi-tenancy. An organization is a tenant: a shared wallet, members
// with roles (owner/admin/member) and org API keys that bill the org wallet.

const (
	OrgRoleOwner  = "owner"
	OrgRoleAdmin  = "admin"
	OrgRoleMember = "member"
)

// ErrOrgMemberExists is returned when adding a duplicate member.
var ErrOrgMemberExists = errors.New("org member exists")

// Organization is one tenant with its own wallet.
type Organization struct {
	ID           int64
	Name         string
	OwnerUserID  int64
	BalanceMicro int64
	CreatedAt    time.Time
}

// OrgMember is a user's membership in an org.
type OrgMember struct {
	OrgID    int64
	UserID   int64
	Email    string
	Role     string
	JoinedAt time.Time
}

const orgColumns = `id, name, owner_user_id, balance, created_at`

func scanOrg(row pgx.Row) (*Organization, error) {
	o := &Organization{}
	err := row.Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.BalanceMicro, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

// CreateOrganization creates an org and inserts the creator as its owner
// member in one transaction.
func (s *Store) CreateOrganization(ctx context.Context, name string, ownerUserID int64) (*Organization, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	org := &Organization{}
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name, owner_user_id) VALUES ($1, $2)
		RETURNING `+orgColumns, name, ownerUserID).
		Scan(&org.ID, &org.Name, &org.OwnerUserID, &org.BalanceMicro, &org.CreatedAt)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, $3)`,
		org.ID, ownerUserID, OrgRoleOwner); err != nil {
		return nil, err
	}
	return org, tx.Commit(ctx)
}

// GetOrganization fetches one org.
func (s *Store) GetOrganization(ctx context.Context, id int64) (*Organization, error) {
	return scanOrg(s.pool.QueryRow(ctx, `SELECT `+orgColumns+` FROM organizations WHERE id = $1`, id))
}

// OrgWithRole is an org row plus the requesting user's role in it.
type OrgWithRole struct {
	Organization
	Role string
}

// ListOrganizationsByUser lists the orgs the user belongs to, with role.
func (s *Store) ListOrganizationsByUser(ctx context.Context, userID int64) ([]OrgWithRole, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, o.name, o.owner_user_id, o.balance, o.created_at, m.role
		FROM organizations o
		JOIN org_members m ON m.org_id = o.id AND m.user_id = $1
		ORDER BY o.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgWithRole{}
	for rows.Next() {
		var o OrgWithRole
		if err := rows.Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.BalanceMicro, &o.CreatedAt, &o.Role); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// GetOrgMember fetches one membership (404-ish ErrNotFound when the user is
// not in the org).
func (s *Store) GetOrgMember(ctx context.Context, orgID, userID int64) (*OrgMember, error) {
	m := &OrgMember{}
	err := s.pool.QueryRow(ctx, `
		SELECT m.org_id, m.user_id, u.email, m.role, m.joined_at
		FROM org_members m JOIN users u ON u.id = m.user_id
		WHERE m.org_id = $1 AND m.user_id = $2`, orgID, userID).
		Scan(&m.OrgID, &m.UserID, &m.Email, &m.Role, &m.JoinedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// ListOrgMembers lists an org's members, oldest first.
func (s *Store) ListOrgMembers(ctx context.Context, orgID int64) ([]OrgMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.org_id, m.user_id, u.email, m.role, m.joined_at
		FROM org_members m JOIN users u ON u.id = m.user_id
		WHERE m.org_id = $1 ORDER BY m.joined_at, m.user_id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgMember{}
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.OrgID, &m.UserID, &m.Email, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddOrgMember adds a user (by email) to an org with the given role.
func (s *Store) AddOrgMember(ctx context.Context, orgID int64, email, role string) (*OrgMember, error) {
	var userID int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m := &OrgMember{}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, $3)
		RETURNING org_id, user_id`, orgID, userID, role).Scan(&m.OrgID, &m.UserID)
	if err != nil {
		var pgxErr *pgconn.PgError
		if errors.As(err, &pgxErr) && pgxErr.Code == "23505" {
			return nil, ErrOrgMemberExists
		}
		return nil, err
	}
	m.Role = role
	m.Email = email
	return m, nil
}

// SetOrgMemberRole changes a member's role (the owner role is immutable).
func (s *Store) SetOrgMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE org_members SET role = $3 WHERE org_id = $1 AND user_id = $2 AND role != 'owner'`, orgID, userID, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// either not a member, or the owner (immutable)
		var exists bool
		err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_members WHERE org_id = $1 AND user_id = $2)`, orgID, userID).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		return fmt.Errorf("the owner role cannot be changed")
	}
	return nil
}

// RemoveOrgMember removes a member (the owner cannot be removed).
func (s *Store) RemoveOrgMember(ctx context.Context, orgID, userID int64) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM org_members WHERE org_id = $1 AND user_id = $2 AND role != 'owner'`, orgID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_members WHERE org_id = $1 AND user_id = $2)`, orgID, userID).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		return fmt.Errorf("the owner cannot be removed")
	}
	return nil
}

// OrgLedgerEntry is one org wallet movement.
type OrgLedgerEntry struct {
	ID        int64
	OrgID     int64
	Kind      string
	Amount    int64
	Reason    string
	RequestID *string
	CreatedAt time.Time
}

// ListOrgLedger lists recent org wallet movements.
func (s *Store) ListOrgLedger(ctx context.Context, orgID int64, limit int) ([]OrgLedgerEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, kind, amount, reason, request_id, created_at
		FROM org_ledger_entries WHERE org_id = $1 ORDER BY id DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgLedgerEntry{}
	for rows.Next() {
		var e OrgLedgerEntry
		if err := rows.Scan(&e.ID, &e.OrgID, &e.Kind, &e.Amount, &e.Reason, &e.RequestID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// OrgUsageRow is one usage-log row for a key of the org.
type OrgUsageRow struct {
	ID              int64
	APIKeyID        *int64
	KeyName         string
	ModelID         string
	Provider        string
	Stream          bool
	PromptTokens    int
	CompletionTokens int
	Cost            float64
	Status          string
	ErrorMsg        string
	CreatedAt       time.Time
}

// ListOrgUsage lists recent usage across the org's keys (join via org_id).
func (s *Store) ListOrgUsage(ctx context.Context, orgID int64, limit int) ([]OrgUsageRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.api_key_id, COALESCE(k.name, ''), u.model_id, u.provider, u.stream,
		       u.prompt_tokens, u.completion_tokens, u.cost, u.status, u.error_msg, u.created_at
		FROM usage_logs u
		JOIN api_keys k ON k.id = u.api_key_id
		WHERE k.org_id = $1 ORDER BY u.id DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgUsageRow{}
	for rows.Next() {
		var r OrgUsageRow
		if err := rows.Scan(&r.ID, &r.APIKeyID, &r.KeyName, &r.ModelID, &r.Provider, &r.Stream,
			&r.PromptTokens, &r.CompletionTokens, &r.Cost, &r.Status, &r.ErrorMsg, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// OrgUsageSummary aggregates the org's usage.
type OrgUsageSummary struct {
	Requests        int
	PromptTokens    int
	CompletionTokens int
	CostUSD         float64
}

// OrgUsageSummary aggregates all usage of the org's keys.
func (s *Store) OrgUsageSummary(ctx context.Context, orgID int64) (*OrgUsageSummary, error) {
	out := &OrgUsageSummary{}
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(u.prompt_tokens), 0), COALESCE(SUM(u.completion_tokens), 0), COALESCE(SUM(u.cost), 0)
		FROM usage_logs u JOIN api_keys k ON k.id = u.api_key_id
		WHERE k.org_id = $1`, orgID).
		Scan(&out.Requests, &out.PromptTokens, &out.CompletionTokens, &out.CostUSD)
	return out, err
}

// AdminOrgRow is an org row for the admin list, with owner email and member count.
type AdminOrgRow struct {
	Organization
	OwnerEmail  string
	MemberCount int
}

// AdminListOrganizations lists all orgs, newest first.
func (s *Store) AdminListOrganizations(ctx context.Context, limit int) ([]AdminOrgRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, o.name, o.owner_user_id, o.balance, o.created_at,
		       u.email,
		       (SELECT COUNT(*) FROM org_members m WHERE m.org_id = o.id)
		FROM organizations o JOIN users u ON u.id = o.owner_user_id
		ORDER BY o.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminOrgRow{}
	for rows.Next() {
		var r AdminOrgRow
		if err := rows.Scan(&r.ID, &r.Name, &r.OwnerUserID, &r.BalanceMicro, &r.CreatedAt, &r.OwnerEmail, &r.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}