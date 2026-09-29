package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// User is a billing account. Balance is denominated in micro-USD and
// mirrors SUM(ledger_entries.amount) for the user. AgentRate is non-nil when
// the user is a reseller agent (see agents table); it is populated by
// GetUser only.
type User struct {
	ID           int64
	Email        string
	PasswordHash *string
	Balance      int64
	Enabled      bool
	CreatedAt    time.Time
	AgentRate    *float64
}

// LedgerEntry is one immutable wallet transaction. Kind is one of
// credit, hold, release, debit; amount is signed micro-USD.
type LedgerEntry struct {
	ID        int64
	UserID    int64
	Kind      string
	Amount    int64
	Reason    string
	RequestID *string
	CreatedAt time.Time
}

const userColumns = `id, email, password_hash, balance, enabled, created_at`

func scanUser(row pgx.Row) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Balance, &u.Enabled, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

// CreateUser inserts a new user. A positive initialBalanceMicro is recorded
// as a ledger credit so the wallet invariant holds from day one.
func (s *Store) CreateUser(ctx context.Context, email string, initialBalanceMicro int64) (*User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	u := &User{}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, balance) VALUES ($1, 0)
		RETURNING `+userColumns, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Balance, &u.Enabled, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if initialBalanceMicro > 0 {
		if _, err = tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, kind, amount, reason)
			VALUES ($1, 'credit', $2, 'initial balance')`, u.ID, initialBalanceMicro); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET balance = balance + $2 WHERE id = $1`, u.ID, initialBalanceMicro); err != nil {
			return nil, err
		}
		u.Balance = initialBalanceMicro
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return u, nil
}

// GetUser fetches one user by ID, including the agent wholesale rate when
// the user has one.
func (s *Store) GetUser(ctx context.Context, id int64) (*User, error) {
	u := &User{}
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.password_hash, u.balance, u.enabled, u.created_at, a.rate
		FROM users u LEFT JOIN agents a ON a.user_id = u.id
		WHERE u.id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Balance, &u.Enabled, &u.CreatedAt, &u.AgentRate)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

// ListUsers returns all users, oldest first.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Balance, &u.Enabled, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CreditUser adds funds to the user's balance together with a ledger credit
// (manual top-up).
func (s *Store) CreditUser(ctx context.Context, id, amountMicro int64, reason string) (*User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	u := &User{}
	err = tx.QueryRow(ctx, `
		UPDATE users SET balance = balance + $2 WHERE id = $1 AND enabled
		RETURNING `+userColumns, id, amountMicro,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Balance, &u.Enabled, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount, reason)
		VALUES ($1, 'credit', $2, $3)`, id, amountMicro, reason); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return u, nil
}

// ListLedger returns the user's wallet history, newest first.
func (s *Store) ListLedger(ctx context.Context, userID int64, limit, offset int) ([]LedgerEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, kind, amount, reason, request_id, created_at
		FROM ledger_entries
		WHERE user_id = $1
		ORDER BY id DESC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LedgerEntry{}
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Kind, &e.Amount, &e.Reason, &e.RequestID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
