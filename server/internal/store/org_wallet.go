package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// P3-3: the organization wallet mirrors the user wallet (wallet.go) against
// organizations.balance and org_ledger_entries. Holds settle all-or-nothing
// against the org balance; org keys bill at list price (no agent rate).

// HoldOrgFunds freezes amountMicro from the org's balance for one request.
// The hold ledger entry is tagged requestID+":hold" for idempotency.
func (s *Store) HoldOrgFunds(ctx context.Context, orgID, amountMicro int64, requestID, reason string) error {
	if amountMicro <= 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var balance int64
	err = tx.QueryRow(ctx, `SELECT balance FROM organizations WHERE id = $1 FOR UPDATE`, orgID).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if balance < amountMicro {
		return ErrInsufficientBalance
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO org_ledger_entries (org_id, kind, amount, reason, request_id)
		VALUES ($1, 'hold', $2, $3, $4)`, orgID, -amountMicro, reason, requestID+":hold"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE organizations SET balance = balance - $2 WHERE id = $1`, orgID, amountMicro); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SettleOrgFunds converts a hold into the actual charge on the org wallet:
// releases holdMicro and debits actualMicro in one transaction. actualMicro
// may exceed holdMicro (overdraft); the ledger stays consistent either way.
// orgID may be nil (quota-only keys): then only the key spend is updated.
func (s *Store) SettleOrgFunds(ctx context.Context, orgID *int64, holdMicro, actualMicro int64, requestID, reason string, keyID *int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if orgID != nil {
		if holdMicro > 0 {
			if _, err = tx.Exec(ctx, `
				INSERT INTO org_ledger_entries (org_id, kind, amount, reason, request_id)
				VALUES ($1, 'release', $2, $3, $4)`, *orgID, holdMicro, reason, requestID+":release"); err != nil {
				return err
			}
		}
		if actualMicro > 0 {
			if _, err = tx.Exec(ctx, `
				INSERT INTO org_ledger_entries (org_id, kind, amount, reason, request_id)
				VALUES ($1, 'debit', -$2, $3, $4)`, *orgID, actualMicro, reason, requestID+":debit"); err != nil {
				return err
			}
		}
		if delta := holdMicro - actualMicro; delta != 0 {
			if _, err = tx.Exec(ctx, `UPDATE organizations SET balance = balance + $2 WHERE id = $1`, *orgID, delta); err != nil {
				return err
			}
		}
	}
	if keyID != nil && actualMicro > 0 {
		if _, err = tx.Exec(ctx, `UPDATE api_keys SET spend = spend + $2 WHERE id = $1`, *keyID, actualMicro); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ReleaseOrgFunds returns a full org hold without charging (request failed).
func (s *Store) ReleaseOrgFunds(ctx context.Context, orgID, holdMicro int64, requestID, reason string) error {
	if holdMicro <= 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, `
		INSERT INTO org_ledger_entries (org_id, kind, amount, reason, request_id)
		VALUES ($1, 'release', $2, $3, $4)`, orgID, holdMicro, reason, requestID+":release"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE organizations SET balance = balance + $2 WHERE id = $1`, orgID, holdMicro); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CreditOrg adds amountMicro to the org balance (admin manual top-up) and
// records a 'credit' ledger entry.
func (s *Store) CreditOrg(ctx context.Context, orgID, amountMicro int64, reason string) (*Organization, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, `
		INSERT INTO org_ledger_entries (org_id, kind, amount, reason)
		VALUES ($1, 'credit', $2, $3)`, orgID, amountMicro, reason); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE organizations SET balance = balance + $2 WHERE id = $1`, orgID, amountMicro); err != nil {
		return nil, err
	}
	org := &Organization{}
	err = tx.QueryRow(ctx, `
		SELECT id, name, owner_user_id, balance, created_at FROM organizations WHERE id = $1`, orgID).
		Scan(&org.ID, &org.Name, &org.OwnerUserID, &org.BalanceMicro, &org.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return org, tx.Commit(ctx)
}