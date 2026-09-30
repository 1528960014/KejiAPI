package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// P0 batch: terms, announcements, redeem/promo codes, subscription plans,
// recharge stats and refunds.

// ---------- terms ----------

// Terms is the single terms-of-service document (row id=1).
type Terms struct {
	Content   string
	UpdatedAt time.Time
}

func (s *Store) GetTerms(ctx context.Context) (*Terms, error) {
	t := &Terms{}
	err := s.pool.QueryRow(ctx, `SELECT content, updated_at FROM system_terms WHERE id = 1`).
		Scan(&t.Content, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return &Terms{}, nil
	}
	return t, err
}

func (s *Store) PutTerms(ctx context.Context, content string) (*Terms, error) {
	t := &Terms{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO system_terms (id, content, updated_at) VALUES (1, $1, now())
		ON CONFLICT (id) DO UPDATE SET content = EXCLUDED.content, updated_at = now()
		RETURNING content, updated_at`, content).Scan(&t.Content, &t.UpdatedAt)
	return t, err
}

// ---------- announcements ----------

type Announcement struct {
	ID        int64
	Title     string
	Content   string
	Style     string // popup | banner
	Active    bool
	Views     int
	CreatedAt time.Time
}

const announcementColumns = `id, title, content, style, active, views, created_at`

func scanAnnouncementRow(row pgx.Row) (*Announcement, error) {
	a := &Announcement{}
	err := row.Scan(&a.ID, &a.Title, &a.Content, &a.Style, &a.Active, &a.Views, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *Store) ListActiveAnnouncements(ctx context.Context) ([]Announcement, error) {
	return s.queryAnnouncements(ctx, `WHERE active ORDER BY id DESC LIMIT 10`)
}

func (s *Store) ListAnnouncements(ctx context.Context) ([]Announcement, error) {
	return s.queryAnnouncements(ctx, `ORDER BY id DESC LIMIT 100`)
}

func (s *Store) queryAnnouncements(ctx context.Context, clause string) ([]Announcement, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+announcementColumns+` FROM announcements `+clause)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Announcement{}
	for rows.Next() {
		var a Announcement
		if err := rows.Scan(&a.ID, &a.Title, &a.Content, &a.Style, &a.Active, &a.Views, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAnnouncement(ctx context.Context, id int64) (*Announcement, error) {
	return scanAnnouncementRow(s.pool.QueryRow(ctx,
		`SELECT `+announcementColumns+` FROM announcements WHERE id = $1`, id))
}

func (s *Store) CreateAnnouncement(ctx context.Context, title, content, style string) (*Announcement, error) {
	return scanAnnouncementRow(s.pool.QueryRow(ctx, `
		INSERT INTO announcements (title, content, style) VALUES ($1, $2, $3)
		RETURNING `+announcementColumns, title, content, style))
}

func (s *Store) UpdateAnnouncement(ctx context.Context, id int64, title, content, style string, active bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE announcements SET title = $2, content = $3, style = $4, active = $5
		WHERE id = $1`, id, title, content, style, active)
	return err
}

func (s *Store) DeleteAnnouncement(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM announcements WHERE id = $1`, id)
	return err
}

func (s *Store) BumpAnnouncementView(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE announcements SET views = views + 1 WHERE id = $1`, id)
	return err
}

// ---------- redeem codes ----------

type RedeemCode struct {
	ID          int64
	Code        string
	CreditMicro int64
	UsedBy      *int64
	UsedAt      *time.Time
	CreatedAt   time.Time
}

func (s *Store) ListRedeemCodes(ctx context.Context) ([]RedeemCode, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, code, credit_micro, used_by, used_at, created_at
		FROM redeem_codes ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RedeemCode{}
	for rows.Next() {
		var r RedeemCode
		if err := rows.Scan(&r.ID, &r.Code, &r.CreditMicro, &r.UsedBy, &r.UsedAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func genRedeemCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "KEJI-" + strings.ToUpper(hex.EncodeToString(b)), nil
}

// CreateRedeemCodes bulk-creates n codes each worth creditMicro.
func (s *Store) CreateRedeemCodes(ctx context.Context, n int, creditMicro int64) ([]RedeemCode, error) {
	if n < 1 {
		n = 1
	}
	if n > 500 {
		n = 500
	}
	out := make([]RedeemCode, 0, n)
	for i := 0; i < n; i++ {
		code, err := genRedeemCode()
		if err != nil {
			return nil, err
		}
		r := RedeemCode{}
		err = s.pool.QueryRow(ctx, `
			INSERT INTO redeem_codes (code, credit_micro) VALUES ($1, $2)
			RETURNING id, code, credit_micro, used_by, used_at, created_at`, code, creditMicro).
			Scan(&r.ID, &r.Code, &r.CreditMicro, &r.UsedBy, &r.UsedAt, &r.CreatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// DeleteRedeemCode removes an unused code (used codes are kept for audit).
func (s *Store) DeleteRedeemCode(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM redeem_codes WHERE id = $1 AND used_at IS NULL`, id)
	return err
}

var ErrRedeemUsed = errors.New("redeem code already used")

// RedeemCode marks the code used by the user and credits the balance.
// The ledger credit is tagged with the code itself (DB-UNIQUE request_id),
// so a double credit is impossible.
func (s *Store) RedeemCode(ctx context.Context, code string, userID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var creditMicro int64
	err = tx.QueryRow(ctx, `
		UPDATE redeem_codes
		SET used_by = $2, used_at = now()
		WHERE code = $1 AND used_at IS NULL
		RETURNING credit_micro`, code, userID).Scan(&creditMicro)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Commit(ctx)
		var exists bool
		if e := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM redeem_codes WHERE code = $1)`, code).Scan(&exists); e == nil && !exists {
			return ErrNotFound
		}
		return ErrRedeemUsed
	}
	if err != nil {
		return err
	}
	if creditMicro > 0 {
		if _, err = tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, kind, amount, reason, request_id)
			VALUES ($1, 'credit', $2, 'redeem code', $3)`, userID, creditMicro, code); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET balance = balance + $2 WHERE id = $1`, userID, creditMicro); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ---------- promo codes ----------

type PromoCode struct {
	ID        int64
	Code      string
	Kind      string // percent | amount_off
	Value     int64
	MinCNYFen int64
	MaxUses   int
	UsedCount int
	ExpiresAt *time.Time
	Enabled   bool
	CreatedAt time.Time
}

const promoColumns = `id, code, kind, value, min_cny_fen, max_uses, used_count, expires_at, enabled, created_at`

func (s *Store) ListPromoCodes(ctx context.Context) ([]PromoCode, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+promoColumns+` FROM promo_codes ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PromoCode{}
	for rows.Next() {
		var p PromoCode
		if err := rows.Scan(&p.ID, &p.Code, &p.Kind, &p.Value, &p.MinCNYFen, &p.MaxUses, &p.UsedCount, &p.ExpiresAt, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPromoCode(ctx context.Context, code string) (*PromoCode, error) {
	p := &PromoCode{}
	err := s.pool.QueryRow(ctx, `SELECT `+promoColumns+` FROM promo_codes WHERE code = $1`, code).
		Scan(&p.ID, &p.Code, &p.Kind, &p.Value, &p.MinCNYFen, &p.MaxUses, &p.UsedCount, &p.ExpiresAt, &p.Enabled, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) CreatePromoCode(ctx context.Context, p *PromoCode) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO promo_codes (code, kind, value, min_cny_fen, max_uses, expires_at, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		p.Code, p.Kind, p.Value, p.MinCNYFen, p.MaxUses, p.ExpiresAt, p.Enabled).Scan(&p.ID)
}

func (s *Store) UpdatePromoCode(ctx context.Context, p *PromoCode) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE promo_codes
		SET kind = $2, value = $3, min_cny_fen = $4, max_uses = $5, expires_at = $6, enabled = $7
		WHERE id = $1`, p.ID, p.Kind, p.Value, p.MinCNYFen, p.MaxUses, p.ExpiresAt, p.Enabled)
	return err
}

func (s *Store) DeletePromoCode(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM promo_codes WHERE id = $1`, id)
	return err
}

// PromoUsable reports whether a promo code can be applied right now.
func (p *PromoCode) PromoUsable(now time.Time) bool {
	if p == nil || !p.Enabled {
		return false
	}
	if p.ExpiresAt != nil && now.After(*p.ExpiresAt) {
		return false
	}
	if p.MaxUses > 0 && p.UsedCount >= p.MaxUses {
		return false
	}
	return true
}

// DiscountedCNYFen computes the payable CNY (fen) after the promo is applied,
// floored at ¥1 so a 100%-style code never produces a free order.
func (p *PromoCode) DiscountedCNYFen(amountCNYFen int64) int64 {
	if p == nil {
		return amountCNYFen
	}
	var off int64
	if p.Kind == "amount_off" {
		off = p.Value
	} else {
		off = amountCNYFen * p.Value / 100
	}
	out := amountCNYFen - off
	if out < 100 {
		out = 100
	}
	return out
}

// UsePromoCode increments used_count when an order is created.
func (s *Store) UsePromoCode(ctx context.Context, code string) error {
	_, err := s.pool.Exec(ctx, `UPDATE promo_codes SET used_count = used_count + 1 WHERE code = $1`, code)
	return err
}

// ---------- plans & subscriptions ----------

type Plan struct {
	ID          int64
	Name        string
	QuotaMicro  int64
	PeriodDays  int
	PriceCNYFen int64
	Enabled     bool
	CreatedAt   time.Time
}

func (s *Store) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, quota_micro, period_days, price_cny_fen, enabled, created_at
		FROM plans ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.QuotaMicro, &p.PeriodDays, &p.PriceCNYFen, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPlan(ctx context.Context, id int64) (*Plan, error) {
	p := &Plan{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, quota_micro, period_days, price_cny_fen, enabled, created_at
		FROM plans WHERE id = $1`, id).
		Scan(&p.ID, &p.Name, &p.QuotaMicro, &p.PeriodDays, &p.PriceCNYFen, &p.Enabled, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) CreatePlan(ctx context.Context, p *Plan) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO plans (name, quota_micro, period_days, price_cny_fen, enabled)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		p.Name, p.QuotaMicro, p.PeriodDays, p.PriceCNYFen, p.Enabled).Scan(&p.ID)
}

func (s *Store) UpdatePlan(ctx context.Context, p *Plan) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE plans SET name = $2, quota_micro = $3, period_days = $4, price_cny_fen = $5, enabled = $6
		WHERE id = $1`, p.ID, p.Name, p.QuotaMicro, p.PeriodDays, p.PriceCNYFen, p.Enabled)
	return err
}

func (s *Store) DeletePlan(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM plans WHERE id = $1`, id)
	return err
}

// Subscription is a user's active usage quota from a plan.
type Subscription struct {
	UserID     int64
	PlanID     int64
	PlanName   string
	QuotaMicro int64
	UsedMicro  int64
	ResetAt    time.Time
	CreatedAt  time.Time
}

// SetUserSubscription activates (or renews) a plan for a user: quota is
// copied from the plan and reset_at = now + period_days.
func (s *Store) SetUserSubscription(ctx context.Context, userID, planID int64) (*Subscription, error) {
	p, err := s.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	sub := &Subscription{}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO user_subscriptions (user_id, plan_id, quota_micro, used_micro, reset_at)
		VALUES ($1, $2, $3, 0, now() + $4 * interval '1 day')
		ON CONFLICT (user_id) DO UPDATE SET
			plan_id = EXCLUDED.plan_id,
			quota_micro = EXCLUDED.quota_micro,
			used_micro = 0,
			reset_at = EXCLUDED.reset_at
		RETURNING user_id, plan_id, quota_micro, used_micro, reset_at, created_at`,
		userID, p.ID, p.QuotaMicro, p.PeriodDays).
		Scan(&sub.UserID, &sub.PlanID, &sub.QuotaMicro, &sub.UsedMicro, &sub.ResetAt, &sub.CreatedAt)
	if err != nil {
		return nil, err
	}
	sub.PlanName = p.Name
	return sub, nil
}

func (s *Store) GetUserSubscription(ctx context.Context, userID int64) (*Subscription, error) {
	sub := &Subscription{}
	err := s.pool.QueryRow(ctx, `
		SELECT s.user_id, s.plan_id, p.name, s.quota_micro, s.used_micro, s.reset_at, s.created_at
		FROM user_subscriptions s JOIN plans p ON p.id = s.plan_id
		WHERE s.user_id = $1`, userID).
		Scan(&sub.UserID, &sub.PlanID, &sub.PlanName, &sub.QuotaMicro, &sub.UsedMicro, &sub.ResetAt, &sub.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sub, err
}

func (s *Store) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.user_id, s.plan_id, p.name, s.quota_micro, s.used_micro, s.reset_at, s.created_at
		FROM user_subscriptions s JOIN plans p ON p.id = s.plan_id
		ORDER BY s.id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Subscription{}
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.UserID, &sub.PlanID, &sub.PlanName, &sub.QuotaMicro, &sub.UsedMicro, &sub.ResetAt, &sub.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// AddSubUsageTx accumulates settled usage onto the user's active
// subscription (no-op when there is none or it is past its reset time).
// Called from SettleFunds inside the same transaction.
func AddSubUsageTx(ctx context.Context, tx pgx.Tx, userID int64, micro int64) {
	if micro <= 0 || userID <= 0 {
		return
	}
	_, _ = tx.Exec(ctx, `
		UPDATE user_subscriptions SET used_micro = used_micro + $2
		WHERE user_id = $1 AND reset_at > now()`, userID, micro)
}

// ---------- recharge stats & refund ----------

type RechargeStats struct {
	TotalMicro int64
	ByDay      []DailyMicro
	ByMethod   []MethodMicro
	TopUsers   []UserSpend
}

type DailyMicro struct {
	Date  string
	Micro int64
}

type MethodMicro struct {
	Method string
	Micro  int64
	Orders int64
}

type UserSpend struct {
	UserID int64
	Email  string
	Micro  int64
}

// RechargeStats aggregates paid orders over the last N days (1..90).
func (s *Store) RechargeStats(ctx context.Context, days int) (*RechargeStats, error) {
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}
	since := time.Now().UTC().AddDate(0, 0, -(days - 1))
	st := &RechargeStats{}

	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(credit_micro), 0)
		FROM recharges WHERE status = 'paid' AND paid_at >= $1`, since).Scan(&st.TotalMicro); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT to_char(paid_at AT TIME ZONE 'UTC', 'YYYY-MM-DD'), sum(credit_micro)
		FROM recharges WHERE status = 'paid' AND paid_at >= $1
		GROUP BY 1 ORDER BY 1`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d DailyMicro
		if err := rows.Scan(&d.Date, &d.Micro); err != nil {
			return nil, err
		}
		st.ByDay = append(st.ByDay, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = s.pool.Query(ctx, `
		SELECT method, sum(credit_micro), count(*)
		FROM recharges WHERE status = 'paid' AND paid_at >= $1
		GROUP BY 1 ORDER BY 2 DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m MethodMicro
		if err := rows.Scan(&m.Method, &m.Micro, &m.Orders); err != nil {
			return nil, err
		}
		st.ByMethod = append(st.ByMethod, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = s.pool.Query(ctx, `
		SELECT r.user_id, u.email, sum(r.credit_micro)
		FROM recharges r JOIN users u ON u.id = r.user_id
		WHERE r.status = 'paid' AND r.paid_at >= $1
		GROUP BY 1, 2 ORDER BY 3 DESC LIMIT 10`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u UserSpend
		if err := rows.Scan(&u.UserID, &u.Email, &u.Micro); err != nil {
			return nil, err
		}
		st.TopUsers = append(st.TopUsers, u)
	}
	return st, rows.Err()
}

var ErrRechargeRefund = errors.New("refund not allowed")

// RefundRecharge flips a paid order to refunded, debits the user's balance
// and writes a 'debit' ledger entry tagged "refund-<order_no>" (idempotent
// via the DB-UNIQUE request_id).
func (s *Store) RefundRecharge(ctx context.Context, id int64, reason string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		userID, creditMicro int64
		orderNo             string
	)
	err = tx.QueryRow(ctx, `
		UPDATE recharges
		SET status = 'refunded', refund_at = now()
		WHERE id = $1 AND status = 'paid'
		RETURNING user_id, credit_micro, order_no`, id).Scan(&userID, &creditMicro, &orderNo)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Commit(ctx)
		return ErrRechargeRefund
	}
	if err != nil {
		return err
	}
	if creditMicro > 0 {
		why := reason
		if why == "" {
			why = "order refund"
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, kind, amount, reason, request_id)
			VALUES ($1, 'debit', -$2, $3, $4)`, userID, creditMicro, why, "refund-"+orderNo); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET balance = balance - $2 WHERE id = $1`, userID, creditMicro); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}