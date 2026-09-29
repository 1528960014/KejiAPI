package store

// Integration tests for the billing wallet. They run only when
// MODELHUB_TEST_DSN is set, e.g.:
//
//	MODELHUB_TEST_DSN=postgres://modelhub:modelhub@127.0.0.1:5432/modelhub go test ./internal/store/
//
// (start the dependencies first with `docker compose up -d` in deploy/).
// The suite drops and recreates the schema, so point it at a disposable
// database.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("MODELHUB_TEST_DSN")
	if dsn == "" {
		fmt.Println("store: MODELHUB_TEST_DSN not set, skipping integration tests")
		os.Exit(0)
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	if _, err := conn.Exec(ctx, `
		DROP TABLE IF EXISTS refresh_tokens, usage_logs, tasks, api_keys, channels, models, ledger_entries, users CASCADE
	`); err != nil {
		fmt.Println("drop schema:", err)
		os.Exit(1)
	}
	if err := conn.Close(ctx); err != nil {
		fmt.Println("close:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := New(context.Background(), os.Getenv("MODELHUB_TEST_DSN"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

func mustCreditUser(t *testing.T, st *Store, email string, micro int64) *User {
	t.Helper()
	ctx := context.Background()
	u, err := st.CreateUser(ctx, email, 0)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	u, err = st.CreditUser(ctx, u.ID, micro, "test top-up")
	if err != nil {
		t.Fatalf("credit user: %v", err)
	}
	return u
}

func ledgerSum(t *testing.T, st *Store, userID int64) int64 {
	t.Helper()
	entries, err := st.ListLedger(context.Background(), userID, 1000, 0)
	if err != nil {
		t.Fatalf("list ledger: %v", err)
	}
	var sum int64
	for _, e := range entries {
		sum += e.Amount
	}
	return sum
}

func TestWalletHoldSettleFlow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	u := mustCreditUser(t, st, "hold-settle@test.local", 100_000)

	if err := st.HoldFunds(ctx, u.ID, 40_000, "req-1", "chat:test"); err != nil {
		t.Fatalf("hold: %v", err)
	}
	if got, err := st.GetUser(ctx, u.ID); err != nil || got.Balance != 60_000 {
		t.Fatalf("balance after hold = %v (%v), want 60000", got, err)
	}

	plain, key, err := st.CreateAPIKey(ctx, "test-key")
	_ = plain
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := st.SettleFunds(ctx, &u.ID, 40_000, 25_000, "req-1", "chat:test", &key.ID); err != nil {
		t.Fatalf("settle: %v", err)
	}
	got, err := st.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.Balance != 75_000 {
		t.Errorf("balance after settle = %d, want 75000", got.Balance)
	}
	if got.Balance != ledgerSum(t, st, u.ID) {
		t.Errorf("balance %d != ledger sum %d", got.Balance, ledgerSum(t, st, u.ID))
	}
	gk, err := st.GetAPIKey(ctx, key.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if gk.Spend != 25_000 {
		t.Errorf("key spend = %d, want 25000", gk.Spend)
	}
}

func TestHoldInsufficientBalance(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	u := mustCreditUser(t, st, "insufficient@test.local", 10_000)

	err := st.HoldFunds(ctx, u.ID, 20_000, "req-2", "chat:test")
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("hold = %v, want ErrInsufficientBalance", err)
	}
	got, _ := st.GetUser(ctx, u.ID)
	if got.Balance != 10_000 {
		t.Errorf("balance = %d, want 10000 (unchanged)", got.Balance)
	}
	entries, _ := st.ListLedger(ctx, u.ID, 100, 0)
	if len(entries) != 1 || entries[0].Kind != "credit" {
		t.Errorf("ledger = %v, want only the initial credit", entries)
	}
}

func TestReleaseOnFailure(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	u := mustCreditUser(t, st, "release@test.local", 50_000)

	if err := st.HoldFunds(ctx, u.ID, 30_000, "req-3", "chat:test"); err != nil {
		t.Fatalf("hold: %v", err)
	}
	if err := st.ReleaseFunds(ctx, u.ID, 30_000, "req-3", "upstream_error"); err != nil {
		t.Fatalf("release: %v", err)
	}
	got, _ := st.GetUser(ctx, u.ID)
	if got.Balance != 50_000 {
		t.Errorf("balance after release = %d, want 50000", got.Balance)
	}
	if got.Balance != ledgerSum(t, st, u.ID) {
		t.Errorf("balance %d != ledger sum %d", got.Balance, ledgerSum(t, st, u.ID))
	}
}

func TestSettleOverdraft(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	u := mustCreditUser(t, st, "overdraft@test.local", 10_000)

	if err := st.HoldFunds(ctx, u.ID, 10_000, "req-4", "chat:test"); err != nil {
		t.Fatalf("hold: %v", err)
	}
	_, key, err := st.CreateAPIKey(ctx, "overdraft-key")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	// actual usage exceeds the hold -> negative balance is allowed
	if err := st.SettleFunds(ctx, &u.ID, 10_000, 40_000, "req-4", "chat:test", &key.ID); err != nil {
		t.Fatalf("settle: %v", err)
	}
	got, _ := st.GetUser(ctx, u.ID)
	if got.Balance != -30_000 {
		t.Errorf("balance = %d, want -30000", got.Balance)
	}
}

func TestSettleQuotaOnlyKey(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_, key, err := st.CreateAPIKey(ctx, "quota-key")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	// no user bound: only the key spend accumulates
	if err := st.SettleFunds(ctx, nil, 0, 12_345, "req-5", "chat:test", &key.ID); err != nil {
		t.Fatalf("settle: %v", err)
	}
	gk, err := st.GetAPIKey(ctx, key.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if gk.Spend != 12_345 {
		t.Errorf("key spend = %d, want 12345", gk.Spend)
	}
}

func TestUpdateAPIKeyBindings(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	u := mustCreditUser(t, st, "bind@test.local", 0)
	_, key, err := st.CreateAPIKey(ctx, "bind-key")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	patch := &KeyPatch{UserID: &u.ID}
	got, err := st.UpdateAPIKey(ctx, key.ID, patch)
	if err != nil {
		t.Fatalf("update key: %v", err)
	}
	if got.UserID == nil || *got.UserID != u.ID {
		t.Errorf("user_id = %v, want %d", got.UserID, u.ID)
	}

	zero := int64(0)
	got, err = st.UpdateAPIKey(ctx, key.ID, &KeyPatch{UserID: &zero})
	if err != nil {
		t.Fatalf("unbind key: %v", err)
	}
	if got.UserID != nil {
		t.Errorf("user_id = %v, want nil", got.UserID)
	}

	missing := int64(999999)
	if _, err := st.UpdateAPIKey(ctx, key.ID, &KeyPatch{UserID: &missing}); !errors.Is(err, ErrNotFound) {
		t.Errorf("bind to missing user = %v, want ErrNotFound", err)
	}
}

func TestUsageSummary(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_, key, err := st.CreateAPIKey(ctx, "usage-key")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := st.LogUsage(ctx, &UsageLog{APIKeyID: &key.ID, ModelID: "m", Provider: "p", PromptTokens: 100, CompletionTokens: 50, Cost: 0.15, Status: "ok"}); err != nil {
		t.Fatalf("log 1: %v", err)
	}
	if err := st.LogUsage(ctx, &UsageLog{APIKeyID: &key.ID, ModelID: "m", Provider: "p", Stream: true, PromptTokens: 200, CompletionTokens: 200, Cost: 0.25, Status: "ok_estimated"}); err != nil {
		t.Fatalf("log 2: %v", err)
	}
	if err := st.LogUsage(ctx, &UsageLog{ModelID: "other", Provider: "p", PromptTokens: 999, CompletionTokens: 999, Cost: 9.99, Status: "ok"}); err != nil {
		t.Fatalf("log 3: %v", err)
	}

	summary, err := st.UsageSummary(ctx, &key.ID, nil)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Requests != 2 || summary.PromptTokens != 300 || summary.CompletionTokens != 250 {
		t.Errorf("summary = %+v, want 2/300/250", summary)
	}
	if summary.CostMicro != 400_000 {
		t.Errorf("cost_micro = %d, want 400000", summary.CostMicro)
	}

	all, err := st.UsageSummary(ctx, nil, nil)
	if err != nil {
		t.Fatalf("summary all: %v", err)
	}
	if all.Requests != 3 {
		t.Errorf("all requests = %d, want 3", all.Requests)
	}
}

func TestRefreshTokenRotation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	u, err := st.CreateAccount(ctx, "refresh@test.local", "hash")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.CreateRefreshToken(ctx, u.ID, "rft_test_token", time.Hour); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	id, err := st.RedeemRefreshToken(ctx, "rft_test_token")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if id != u.ID {
		t.Fatalf("redeem user = %d, want %d", id, u.ID)
	}
	if _, err := st.RedeemRefreshToken(ctx, "rft_test_token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second redeem = %v, want ErrNotFound (single use)", err)
	}
}