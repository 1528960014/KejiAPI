package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrInsufficientBalance is returned when a hold would drive the balance
// negative.
var ErrInsufficientBalance = errors.New("insufficient balance")

// HoldFunds freezes amountMicro from the user's balance for one request.
// The hold ledger entry is tagged requestID+":hold" for idempotency.
func (s *Store) HoldFunds(ctx context.Context, userID, amountMicro int64, requestID, reason string) error {
	if amountMicro <= 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var balance int64
	err = tx.QueryRow(ctx, `SELECT balance FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&balance)
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
		INSERT INTO ledger_entries (user_id, kind, amount, reason, request_id)
		VALUES ($1, 'hold', $2, $3, $4)`, userID, -amountMicro, reason, requestID+":hold"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET balance = balance - $2 WHERE id = $1`, userID, amountMicro); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SettleFunds converts a hold into the actual charge: releases holdMicro and
// debits actualMicro in one transaction, and accumulates the charged key's
// lifetime spend. actualMicro may exceed holdMicro, in which case the balance
// can go negative (overdraft); the ledger stays consistent either way.
// userID may be nil (quota-only keys): then only the key spend is updated.
func (s *Store) SettleFunds(ctx context.Context, userID *int64, holdMicro, actualMicro int64, requestID, reason string, keyID *int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if userID != nil {
		if holdMicro > 0 {
			if _, err = tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, kind, amount, reason, request_id)
				VALUES ($1, 'release', $2, $3, $4)`, *userID, holdMicro, reason, requestID+":release"); err != nil {
				return err
			}
		}
		if actualMicro > 0 {
			if _, err = tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, kind, amount, reason, request_id)
				VALUES ($1, 'debit', -$2, $3, $4)`, *userID, actualMicro, reason, requestID+":debit"); err != nil {
				return err
			}
		}
		if delta := holdMicro - actualMicro; delta != 0 {
			if _, err = tx.Exec(ctx, `UPDATE users SET balance = balance + $2 WHERE id = $1`, *userID, delta); err != nil {
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

// ReleaseFunds returns a full hold without charging (request failed).
func (s *Store) ReleaseFunds(ctx context.Context, userID, holdMicro int64, requestID, reason string) error {
	if holdMicro <= 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount, reason, request_id)
		VALUES ($1, 'release', $2, $3, $4)`, userID, holdMicro, reason, requestID+":release"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET balance = balance + $2 WHERE id = $1`, userID, holdMicro); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
