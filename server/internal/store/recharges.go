package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Recharge is one online top-up order. AmountCNY is in fen (1 CNY = 100 fen);
// CreditMicro is the fixed micro-USD credited to the balance once the
// channel notify verifies (locked at order time, immune to rate changes).
type Recharge struct {
	ID             int64
	UserID         int64
	OrderNo        string
	Method         string // yipay | alipay | wechat
	AmountCNY      int64
	CreditMicro    int64
	ChannelTradeNo string
	Status         string // pending | paid | failed | refunded
	PromoCode      string
	CreatedAt      time.Time
	PaidAt         *time.Time
	RefundAt       *time.Time
	Email          string // populated by AdminListRecharges
}

const rechargeColumns = `id, user_id, order_no, method, amount_cny, credit_micro, channel_trade_no, status, promo_code, created_at, paid_at, refund_at`

func scanRecharge(row pgx.Row) (*Recharge, error) {
	r := &Recharge{}
	err := row.Scan(&r.ID, &r.UserID, &r.OrderNo, &r.Method, &r.AmountCNY, &r.CreditMicro, &r.ChannelTradeNo, &r.Status, &r.PromoCode, &r.CreatedAt, &r.PaidAt, &r.RefundAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

// genOrderNo builds a merchant order number: RH + unix millis + 4 random hex
// chars. Unique enough for a single-instance gateway (also DB-UNIQUE).
func genOrderNo() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("RH%013d%04x", time.Now().UnixMilli(), b), nil
}

// CreateRecharge inserts a pending recharge order. amountCNYFen is the
// payable (post-promo) amount; promoCode may be empty.
func (s *Store) CreateRecharge(ctx context.Context, userID int64, method string, amountCNYFen, creditMicro int64, promoCode string) (*Recharge, error) {
	orderNo, err := genOrderNo()
	if err != nil {
		return nil, err
	}
	r := &Recharge{}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO recharges (user_id, order_no, method, amount_cny, credit_micro, promo_code)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+rechargeColumns, userID, orderNo, method, amountCNYFen, creditMicro, promoCode,
	).Scan(&r.ID, &r.UserID, &r.OrderNo, &r.Method, &r.AmountCNY, &r.CreditMicro, &r.ChannelTradeNo, &r.Status, &r.PromoCode, &r.CreatedAt, &r.PaidAt, &r.RefundAt)
	return r, err
}

// GetRechargeByOrderNo fetches one order by its merchant order number.
func (s *Store) GetRechargeByOrderNo(ctx context.Context, orderNo string) (*Recharge, error) {
	return scanRecharge(s.pool.QueryRow(ctx,
		`SELECT `+rechargeColumns+` FROM recharges WHERE order_no = $1`, orderNo))
}

// GetRecharge fetches one of a user's own orders.
func (s *Store) GetRecharge(ctx context.Context, id, userID int64) (*Recharge, error) {
	return scanRecharge(s.pool.QueryRow(ctx,
		`SELECT `+rechargeColumns+` FROM recharges WHERE id = $1 AND user_id = $2`, id, userID))
}

// ListRecharges returns a user's recent orders, newest first.
func (s *Store) ListRecharges(ctx context.Context, userID int64, limit int) ([]Recharge, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+rechargeColumns+` FROM recharges
		WHERE user_id = $1 ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recharge{}
	for rows.Next() {
		var r Recharge
		if err := rows.Scan(&r.ID, &r.UserID, &r.OrderNo, &r.Method, &r.AmountCNY, &r.CreditMicro, &r.ChannelTradeNo, &r.Status, &r.PromoCode, &r.CreatedAt, &r.PaidAt, &r.RefundAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AdminListRecharges lists orders across users (newest first) with emails.
func (s *Store) AdminListRecharges(ctx context.Context, limit, offset int) ([]Recharge, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+rechargeColumns+`, u.email
		FROM recharges r JOIN users u ON u.id = r.user_id
		ORDER BY r.id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recharge{}
	for rows.Next() {
		var r Recharge
		if err := rows.Scan(&r.ID, &r.UserID, &r.OrderNo, &r.Method, &r.AmountCNY, &r.CreditMicro, &r.ChannelTradeNo, &r.Status, &r.PromoCode, &r.CreatedAt, &r.PaidAt, &r.RefundAt, &r.Email); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkRechargePaid flips a pending order to paid and credits the user's
// balance in one transaction. It returns false when the order was not
// pending (duplicate notify), which is safe to treat as success. The
// ledger credit is tagged with order_no, which is DB-UNIQUE in
// ledger_entries(request_id), so a double credit is impossible.
func (s *Store) MarkRechargePaid(ctx context.Context, orderNo, channelTradeNo string, creditMicro int64) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var r Recharge
	err = tx.QueryRow(ctx, `
		UPDATE recharges
		SET status = 'paid', paid_at = now(), channel_trade_no = $2
		WHERE order_no = $1 AND status = 'pending'
		RETURNING `+rechargeColumns, orderNo, channelTradeNo,
	).Scan(&r.ID, &r.UserID, &r.OrderNo, &r.Method, &r.AmountCNY, &r.CreditMicro, &r.ChannelTradeNo, &r.Status, &r.CreatedAt, &r.PaidAt)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Commit(ctx)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if creditMicro > 0 {
		if _, err = tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, kind, amount, reason, request_id)
			VALUES ($1, 'credit', $2, 'online recharge', $3)`, r.UserID, creditMicro, orderNo); err != nil {
			return false, err
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET balance = balance + $2 WHERE id = $1`, r.UserID, creditMicro); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// FailRecharge marks a pending order failed (channel create-order error).
func (s *Store) FailRecharge(ctx context.Context, orderNo string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE recharges SET status = 'failed' WHERE order_no = $1 AND status = 'pending'`, orderNo)
	return err
}
