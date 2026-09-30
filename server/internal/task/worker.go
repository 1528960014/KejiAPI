package task

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"kejiapi/internal/billing"
	"kejiapi/internal/gateway"
	"kejiapi/internal/store"
)

// PollInterval is how often the worker looks for queued tasks.
const PollInterval = 2 * time.Second

// TaskTimeout bounds a single task's upstream processing (including async
// polling).
const TaskTimeout = 10 * time.Minute

// Worker pulls queued media tasks off the database and runs them through the
// provider adapters. At v1 scale a DB-poll loop (no external queue) is the
// simplest crash-safe design: a task claimed by a dead worker is failed and
// its hold released at the next startup.
type Worker struct {
	store    *store.Store
	provider *gateway.Provider
	health   *gateway.ChannelHealth // P6-1: shared channel failure cooldown
}

// NewWorker builds a media task worker.
func NewWorker(st *store.Store, p *gateway.Provider, health *gateway.ChannelHealth) *Worker {
	return &Worker{store: st, provider: p, health: health}
}

// Run blocks until ctx is cancelled. It first recovers tasks left running
// by a previous process, then polls for queued work.
func (w *Worker) Run(ctx context.Context) {
	w.recoverStale(ctx)
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.processNext(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("process task", "error", err)
			}
		}
	}
}

// recoverStale fails tasks a dead worker left running and releases their
// holds so funds are never frozen by a crash.
func (w *Worker) recoverStale(ctx context.Context) {
	tasks, err := w.store.ListRunningTasks(ctx, 1000)
	if err != nil {
		slog.Warn("recover stale tasks", "error", err)
		return
	}
	for i := range tasks {
		t := &tasks[i]
		w.failAndRelease(ctx, t, "worker restarted; task interrupted")
		slog.Warn("recovered stale task", "task", t.TaskUUID, "type", t.Type)
	}
}

func (w *Worker) processNext(ctx context.Context) error {
	t, err := w.store.ClaimNextQueued(ctx)
	if err != nil {
		if err == store.ErrNotFound {
			return nil
		}
		return err
	}
	w.runTask(ctx, t)
	return nil
}

func (w *Worker) runTask(ctx context.Context, t *store.Task) {
	reason := "media:" + t.ModelID + ":" + t.Type

	model, err := w.store.GetModel(ctx, t.ModelID)
	if err != nil || !model.Enabled {
		w.failAndRelease(ctx, t, "model no longer available: "+t.ModelID)
		return
	}
	// P6-1: try the model's channels in priority order. One total timeout
	// budget across all attempts; availability failures fail over, request/
	// content failures end the task immediately.
	channels, err := w.store.ChannelsForModel(ctx, t.ModelID)
	if err != nil {
		w.failAndRelease(ctx, t, "no enabled channel for model "+t.ModelID)
		return
	}

	runCtx, cancel := context.WithTimeout(ctx, TaskTimeout)
	defer cancel()

	now := time.Now()
	var lastErr error
	for _, ch := range channels {
		if w.health.IsDown(ch.ID, now) {
			continue
		}
		ad := adapterFor(ch.Provider, t.Type)
		if ad == nil {
			lastErr = fmt.Errorf("provider %q does not support %q generation yet", ch.Provider, t.Type)
			continue
		}
		urls, rErr := ad.Run(runCtx, w.provider, ch, model, t.Payload)
		if rErr == nil {
			w.health.MarkOK(ch.ID)
			w.completeTask(ctx, t, reason, urls)
			return
		}
		if !taskFailureRetryable(rErr) {
			w.failAndRelease(ctx, t, rErr.Error())
			return
		}
		w.health.MarkFailed(ch.ID, now)
		lastErr = fmt.Errorf("channel %s: %w", ch.Name, rErr)
		slog.Warn("task channel failed, failing over",
			"task", t.TaskUUID, "channel", ch.Name, "error", rErr)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no enabled (non-cooled-down) channel for model %s", t.ModelID)
	}
	w.failAndRelease(ctx, t, lastErr.Error())
}

func (w *Worker) completeTask(ctx context.Context, t *store.Task, reason string, urls []string) {
	sc, cancel2 := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel2()
	if err := w.store.CompleteTask(sc, t.TaskUUID, urls, billing.USD(t.HoldMicro)); err != nil {
		slog.Error("complete task", "task", t.TaskUUID, "error", err)
		return
	}
	var keyID *int64
	if t.APIKeyID != nil {
		keyID = t.APIKeyID
	}
	if t.KeyOrgID != nil {
		if err := w.store.SettleOrgFunds(sc, t.KeyOrgID, t.HoldMicro, t.HoldMicro, t.TaskUUID, reason, keyID); err != nil {
			slog.Error("settle task org funds", "task", t.TaskUUID, "error", err)
		}
	} else {
		if err := w.store.SettleFunds(sc, t.KeyUserID, t.HoldMicro, t.HoldMicro, t.TaskUUID, reason, keyID); err != nil {
			slog.Error("settle task funds", "task", t.TaskUUID, "error", err)
		}
	}
}

// failAndRelease marks the task failed and, when funds were held, releases
// them (no charge on failure).
func (w *Worker) failAndRelease(ctx context.Context, t *store.Task, msg string) {
	sc, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := w.store.FailTask(sc, t.TaskUUID, msg); err != nil {
		if err != store.ErrNotFound {
			slog.Error("fail task", "task", t.TaskUUID, "error", err)
		}
		return
	}
	if t.HoldMicro > 0 {
		reason := "media:" + t.ModelID + ":" + t.Type
		if t.KeyOrgID != nil {
			if err := w.store.ReleaseOrgFunds(sc, *t.KeyOrgID, t.HoldMicro, t.TaskUUID, reason); err != nil {
				slog.Error("release task org funds", "task", t.TaskUUID, "error", err)
			}
		} else if t.KeyUserID != nil {
			if err := w.store.ReleaseFunds(sc, *t.KeyUserID, t.HoldMicro, t.TaskUUID, reason); err != nil {
				slog.Error("release task funds", "task", t.TaskUUID, "error", err)
			}
		}
	}
}
