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
		DROP TABLE IF EXISTS agents, assistants, dramas, recharges, refresh_tokens, usage_logs, tasks, api_keys, channels, models, ledger_entries, users CASCADE
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

func TestTaskLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	u := mustCreditUser(t, st, "tasks@test.local", 0)
	_, key, err := st.CreateAPIKey(ctx, "task-key")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if _, err := st.UpdateAPIKey(ctx, key.ID, &KeyPatch{UserID: &u.ID}); err != nil {
		t.Fatalf("bind key: %v", err)
	}

	if err := st.CreateTask(ctx, &Task{
		TaskUUID: "task-1", APIKeyID: &key.ID, Type: "image",
		ModelID: "flux", Payload: []byte(`{"prompt":"cat"}`), HoldMicro: 200_000,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	claimed, err := st.ClaimNextQueued(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.TaskUUID != "task-1" || claimed.Status != "running" {
		t.Fatalf("claimed = %+v, want running task-1", claimed)
	}
	if claimed.KeyUserID == nil || *claimed.KeyUserID != u.ID {
		t.Fatalf("key user = %v, want %d", claimed.KeyUserID, u.ID)
	}
	// nothing left in the queue
	if _, err := st.ClaimNextQueued(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second claim = %v, want ErrNotFound", err)
	}

	if err := st.CompleteTask(ctx, "task-1", []string{"https://cdn/a.png"}, 0.2); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := st.GetTaskByUUID(ctx, "task-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != "succeeded" || len(got.ResultURLs) != 1 || got.Cost != 0.2 {
		t.Errorf("task = %+v, want succeeded with url and cost 0.2", got)
	}
	// completing twice is a no-op error
	if err := st.CompleteTask(ctx, "task-1", nil, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("second complete = %v, want ErrNotFound", err)
	}

	// failing a succeeded task is a no-op error
	if err := st.FailTask(ctx, "task-1", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("fail succeeded task = %v, want ErrNotFound", err)
	}

	// a running task shows up in the recovery list
	if err := st.CreateTask(ctx, &Task{TaskUUID: "task-2", Type: "video", ModelID: "wan", Payload: []byte(`{}`), HoldMicro: 1}); err != nil {
		t.Fatalf("create task2: %v", err)
	}
	if _, err := st.ClaimNextQueued(ctx); err != nil {
		t.Fatalf("claim task2: %v", err)
	}
	running, err := st.ListRunningTasks(ctx, 100)
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(running) != 1 || running[0].TaskUUID != "task-2" {
		t.Errorf("running = %+v, want [task-2]", running)
	}
}

func TestDramaLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	u := mustCreditUser(t, st, "drama@test.local", 0)
	if err := st.CreateModel(ctx, &Model{
		ModelID: "flux-d", Provider: "openai", UpstreamModel: "flux",
		Capabilities: []string{"image"}, PriceUnit: "image", UnitPrice: 0.02,
	}); err != nil {
		t.Fatalf("create model: %v", err)
	}
	_, key, err := st.CreateAPIKey(ctx, "drama-key")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if _, err := st.UpdateAPIKey(ctx, key.ID, &KeyPatch{UserID: &u.ID}); err != nil {
		t.Fatalf("bind key: %v", err)
	}

	tts := "cosyvoice-v1"
	if err := st.CreateDrama(ctx, &Drama{
		DramaUUID: "drama-1", APIKeyID: &key.ID, Title: "t", Script: "s",
		ImageModel: "flux-d", TTSModel: &tts, ShotsPlanned: 4,
		HoldMicro: 100_000, Shots: []byte("[]"),
	}); err != nil {
		t.Fatalf("create drama: %v", err)
	}

	claimed, err := st.ClaimNextQueuedDrama(ctx)
	if err != nil {
		t.Fatalf("claim drama: %v", err)
	}
	if claimed.DramaUUID != "drama-1" || claimed.Status != "running" {
		t.Fatalf("claimed = %+v, want running drama-1", claimed)
	}
	if claimed.KeyUserID == nil || *claimed.KeyUserID != u.ID {
		t.Fatalf("drama key user = %v, want %d", claimed.KeyUserID, u.ID)
	}
	if _, err := st.ClaimNextQueuedDrama(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second claim = %v, want ErrNotFound", err)
	}

	// progress updates only apply while running
	if err := st.UpdateDramaShots(ctx, "drama-1", []byte(`[{"shot_no":1,"status":"succeeded","image_url":"https://cdn/a.png"}]`)); err != nil {
		t.Fatalf("update shots: %v", err)
	}
	if err := st.CompleteDrama(ctx, "drama-1", 0.1); err != nil {
		t.Fatalf("complete drama: %v", err)
	}
	got, err := st.GetDramaByUUID(ctx, "drama-1")
	if err != nil {
		t.Fatalf("get drama: %v", err)
	}
	if got.Status != "succeeded" || got.Cost != 0.1 {
		t.Errorf("drama = %+v, want succeeded cost 0.1", got)
	}
	if !contains(got.Shots, "cdn/a.png") {
		t.Errorf("shots = %s, want to contain the result url", got.Shots)
	}
	// finishing twice is a no-op error
	if err := st.CompleteDrama(ctx, "drama-1", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("second complete = %v, want ErrNotFound", err)
	}
	if err := st.FailDrama(ctx, "drama-1", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("fail succeeded drama = %v, want ErrNotFound", err)
	}

	// a running drama shows up in the recovery list
	if err := st.CreateDrama(ctx, &Drama{DramaUUID: "drama-2", ImageModel: "flux-d", ShotsPlanned: 2, HoldMicro: 1, Shots: []byte("[]")}); err != nil {
		t.Fatalf("create drama2: %v", err)
	}
	if _, err := st.ClaimNextQueuedDrama(ctx); err != nil {
		t.Fatalf("claim drama2: %v", err)
	}
	running, err := st.ListRunningDramas(ctx, 100)
	if err != nil {
		t.Fatalf("list running dramas: %v", err)
	}
	if len(running) != 1 || running[0].DramaUUID != "drama-2" {
		t.Errorf("running = %+v, want [drama-2]", running)
	}
}

func TestAgentAndSubkeys(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	u := mustCreditUser(t, st, "agent@test.local", 1_000_000)

	// rate validation
	if _, err := st.CreateAgent(ctx, u.ID, 1.5); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("create agent rate 1.5 = %v, want ErrInvalidRate", err)
	}
	a, err := st.CreateAgent(ctx, u.ID, 0.85)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// creating a second agency for the same user violates the unique index
	if _, err := st.CreateAgent(ctx, u.ID, 0.9); err == nil {
		t.Fatalf("second agent for same user = nil, want duplicate-key error")
	}
	// GetUser now surfaces the wholesale rate
	got, err := st.GetUser(ctx, u.ID)
	if err != nil || got.AgentRate == nil || *got.AgentRate != 0.85 {
		t.Fatalf("get user rate = %v (%v), want 0.85", got, err)
	}

	// a subkey bills the agent user and carries the agent id + markup
	plain, sub, err := st.CreateSubkey(ctx, a.ID, u.ID, "customer-a", 1.2, []string{"gpt-x"}, nil, nil)
	if err != nil {
		t.Fatalf("create subkey: %v", err)
	}
	if plain == "" || sub.AgentID == nil || *sub.AgentID != a.ID || *sub.UserID != u.ID {
		t.Fatalf("subkey = %+v, want agent %d bound to user %d", sub, a.ID, u.ID)
	}
	if sub.Markup != 1.2 {
		t.Errorf("markup = %v, want 1.2", sub.Markup)
	}
	// the agent's own key list must not include subkeys
	_, own, err := st.CreateAPIKeyForUser(ctx, u.ID, "own-key")
	if err != nil {
		t.Fatalf("create own key: %v", err)
	}
	ownKeys, err := st.ListAPIKeysByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("list own keys: %v", err)
	}
	if len(ownKeys) != 1 || ownKeys[0].ID != own.ID {
		t.Fatalf("own keys = %+v, want only [own-key]", ownKeys)
	}
	// the agent cannot delete a subkey as if it were a plain key
	if err := st.DeleteAPIKeyByUser(ctx, u.ID, sub.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete subkey via user path = %v, want ErrNotFound", err)
	}
	subs, err := st.ListSubkeys(ctx, a.ID)
	if err != nil || len(subs) != 1 {
		t.Fatalf("list subkeys = %+v (%v), want 1", subs, err)
	}
	// subkey usage settles against the agent balance and the subkey's own spend
	if err := st.SettleFunds(ctx, &u.ID, 0, 85_000, "req-agent-1", "chat:gpt-x", &sub.ID); err != nil {
		t.Fatalf("settle subkey usage: %v", err)
	}
	gk, err := st.GetAPIKey(ctx, sub.ID)
	if err != nil {
		t.Fatalf("get subkey: %v", err)
	}
	if gk.Spend != 85_000 {
		t.Errorf("subkey spend = %d, want 85000", gk.Spend)
	}

	// rate update
	a2, err := st.UpdateAgentRate(ctx, a.ID, 0.8)
	if err != nil || a2.Rate != 0.8 {
		t.Fatalf("update rate = %+v (%v), want 0.8", a2, err)
	}
	got, _ = st.GetUser(ctx, u.ID)
	if got.AgentRate == nil || *got.AgentRate != 0.8 {
		t.Fatalf("rate after update = %v, want 0.8", got.AgentRate)
	}

	// removing the agency keeps the subkey as a regular key of the agent user
	if err := st.DeleteAgent(ctx, a.ID); err != nil {
		t.Fatalf("delete agent: %v", err)
	}
	got, _ = st.GetUser(ctx, u.ID)
	if got.AgentRate != nil {
		t.Fatalf("rate after agency removal = %v, want nil", got.AgentRate)
	}
	sub2, err := st.GetAPIKey(ctx, sub.ID)
	if err != nil {
		t.Fatalf("get subkey after agency removal: %v", err)
	}
	if sub2.AgentID != nil {
		t.Fatalf("subkey agent_id = %v, want NULL after agency removal", sub2.AgentID)
	}
	if err := st.DeleteSubkey(ctx, a.ID, sub.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete orphaned subkey = %v, want ErrNotFound (no longer a subkey)", err)
	}

	// the agent's own key can still be deleted through the user path
	if err := st.DeleteAPIKeyByUser(ctx, u.ID, own.ID); err != nil {
		t.Fatalf("delete own key: %v", err)
	}
}

func TestAssistants(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	if err := st.CreateModel(ctx, &Model{
		ModelID: "gpt-a", Provider: "openai", UpstreamModel: "gpt-4o-mini",
		Capabilities: []string{"chat"},
	}); err != nil {
		t.Fatalf("create model: %v", err)
	}

	// the bound model must exist
	if _, err := st.CreateAssistant(ctx, &Assistant{AgentID: "agent-x", Name: "X", SystemPrompt: "p", ModelID: "missing"}); err == nil {
		t.Fatalf("create with missing model = nil, want error")
	}

	a, err := st.CreateAssistant(ctx, &Assistant{AgentID: "agent-x", Name: "X", Description: "d", SystemPrompt: "p", ModelID: "gpt-a", Enabled: true})
	if err != nil {
		t.Fatalf("create assistant: %v", err)
	}
	// duplicate agent_id is rejected
	if _, err := st.CreateAssistant(ctx, &Assistant{AgentID: "agent-x", Name: "Y", SystemPrompt: "p", ModelID: "gpt-a"}); err == nil {
		t.Fatalf("duplicate agent_id = nil, want error")
	}

	byID, err := st.GetAssistantByID(ctx, "agent-x")
	if err != nil || byID.ID != a.ID || byID.ModelID != "gpt-a" {
		t.Fatalf("get by id = %+v (%v), want the created row", byID, err)
	}
	if _, err := st.GetAssistantByID(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing = %v, want ErrNotFound", err)
	}

	// list: only enabled ones are public
	all, err := st.ListAssistants(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list all = %+v (%v), want 1", all, err)
	}
	enabled, err := st.ListEnabledAssistants(ctx)
	if err != nil || len(enabled) != 1 {
		t.Fatalf("list enabled = %+v (%v), want 1", enabled, err)
	}

	// partial update, including a model rebind
	name := "X2"
	updated, err := st.UpdateAssistant(ctx, a.ID, &AssistantPatch{Name: &name})
	if err != nil || updated.Name != "X2" || updated.ModelID != "gpt-a" {
		t.Fatalf("update = %+v (%v), want renamed, same model", updated, err)
	}
	if err := st.CreateModel(ctx, &Model{ModelID: "gpt-b", Provider: "openai", UpstreamModel: "gpt-4o", Capabilities: []string{"chat"}}); err != nil {
		t.Fatalf("create model2: %v", err)
	}
	model := "gpt-b"
	updated, err = st.UpdateAssistant(ctx, a.ID, &AssistantPatch{ModelID: &model})
	if err != nil || updated.ModelID != "gpt-b" {
		t.Fatalf("rebind = %+v (%v), want gpt-b", updated, err)
	}
	badModel := "missing"
	if _, err := st.UpdateAssistant(ctx, a.ID, &AssistantPatch{ModelID: &badModel}); err == nil {
		t.Fatalf("rebind to missing model = nil, want error")
	}

	// disable -> hidden from the public list
	disabled := false
	if _, err := st.UpdateAssistant(ctx, a.ID, &AssistantPatch{Enabled: &disabled}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	enabled, err = st.ListEnabledAssistants(ctx)
	if err != nil || len(enabled) != 0 {
		t.Fatalf("list enabled after disable = %+v (%v), want 0", enabled, err)
	}
	all, _ = st.ListAssistants(ctx)
	if len(all) != 1 || all[0].Enabled {
		t.Fatalf("list all = %+v, want 1 disabled row", all)
	}

	if err := st.DeleteAssistant(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := st.DeleteAssistant(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

func TestRecharges(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	u, err := st.CreateUser(ctx, "recharge@example.com", 0)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// create + fetch
	r, err := st.CreateRecharge(ctx, u.ID, "alipay", 10000, 13888889)
	if err != nil {
		t.Fatalf("create recharge: %v", err)
	}
	if r.Status != "pending" || r.OrderNo == "" || r.OrderNo[:2] != "RH" {
		t.Fatalf("bad order: %+v", r)
	}
	byNo, err := st.GetRechargeByOrderNo(ctx, r.OrderNo)
	if err != nil || byNo.ID != r.ID {
		t.Fatalf("by order no: %+v (%v)", byNo, err)
	}

	// other users cannot fetch it
	other, _ := st.CreateUser(ctx, "other@example.com", 0)
	if _, err := st.GetRecharge(ctx, r.ID, other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user get = %v, want ErrNotFound", err)
	}

	// first mark pays, duplicate is a no-op (idempotent notify)
	paid, err := st.MarkRechargePaid(ctx, r.OrderNo, "CH123", 13888889)
	if err != nil || !paid {
		t.Fatalf("mark paid = %v (%v), want true", paid, err)
	}
	paid, err = st.MarkRechargePaid(ctx, r.OrderNo, "CH123", 13888889)
	if err != nil || paid {
		t.Fatalf("duplicate mark = %v (%v), want false (idempotent)", paid, err)
	}

	// balance credited exactly once
	after, err := st.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if after.Balance != 13888889 {
		t.Fatalf("balance = %d, want 13888889 (credited once)", after.Balance)
	}

	// a second order fails cleanly (channel create-order error path)
	f, err := st.CreateRecharge(ctx, u.ID, "wechat", 500, 69444)
	if err != nil {
		t.Fatalf("create failed-order: %v", err)
	}
	if err := st.FailRecharge(ctx, f.OrderNo); err != nil {
		t.Fatalf("fail: %v", err)
	}
	got, _ := st.GetRechargeByOrderNo(ctx, f.OrderNo)
	if got.Status != "failed" {
		t.Fatalf("status = %s, want failed", got.Status)
	}

	// listing
	list, err := st.ListRecharges(ctx, u.ID, 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %d (%v), want 2", len(list), err)
	}
	admin, err := st.AdminListRecharges(ctx, 10, 0)
	if err != nil || len(admin) != 2 || admin[0].Email == "" {
		t.Fatalf("admin list = %+v (%v)", admin, err)
	}
}

func contains(b []byte, sub string) bool {
	return len(b) > 0 && len(sub) > 0 && (len(b) < len(sub) || indexOf(b, sub) >= 0)
}

func indexOf(b []byte, sub string) int {
	for i := 0; i+len(sub) <= len(b); i++ {
		if string(b[i:i+len(sub)]) == sub {
			return i
		}
	}
	return -1
}
