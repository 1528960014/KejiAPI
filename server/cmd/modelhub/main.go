package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"modelhub/internal/api"
	"modelhub/internal/config"
	"modelhub/internal/gateway"
	"modelhub/internal/store"
	"modelhub/internal/task"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("init store", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.EnsureSeedModel(ctx); err != nil {
		slog.Warn("seed model", "error", err)
	}

	provider := gateway.NewProvider()
	// P5-1/P6-1: one shared channel-health tracker so a channel that fails a
	// chat request also cools down before the task workers retry it.
	health := gateway.NewChannelHealth()
	srv := api.New(cfg, st, provider, health)

	// P3-2: seed the payment config from env (first start only), then make the
	// database the runtime source of truth (hot-reloadable via /admin/pay-config).
	if err := srv.ReloadPayConfig(ctx); err != nil {
		slog.Warn("pay config: keeping environment defaults", "error", err)
	}

	// P2-1b: composed drama videos live under <MediaDir>/dramas/.
	if err := os.MkdirAll(filepath.Join(cfg.MediaDir, "dramas"), 0o755); err != nil {
		slog.Warn("create media dir", "dir", cfg.MediaDir, "error", err)
	}

	// M4: media task worker (DB-poll loop; recovers stale tasks on start).
	worker := task.NewWorker(st, provider, health)
	go worker.Run(ctx)

	// P2-1: comic-drama worker (storyboard + per-shot media fan-out).
	dramaWorker := task.NewDramaWorker(st, provider, task.NewComposer(cfg.MediaDir), health)
	go dramaWorker.Run(ctx)

	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      srv.Engine(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("modelhub listening", "port", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown", "error", err)
	}
	slog.Info("modelhub stopped")
}