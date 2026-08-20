package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"t-invest-bot/internal/config"
	"t-invest-bot/internal/operations"
	"t-invest-bot/internal/storage"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("trader stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	state := operations.NewState(cfg.RequestedMode, cfg.EffectiveMode(), time.Now())
	state.SetConfigValid(true)

	db, err := storage.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	state.SetDatabase(true)

	if err := storage.RequireMigration(ctx, db, cfg.RequiredMigration); err != nil {
		return err
	}
	state.SetMigrations(true)

	logger.Info("trader initialized",
		"environment", cfg.Environment,
		"requested_mode", cfg.RequestedMode,
		"effective_mode", cfg.EffectiveMode(),
	)

	operationsServer := newHTTPServer(cfg.HTTPAddr, operations.Handler(state))
	metricsServer := newHTTPServer(cfg.MetricsAddr, operations.MetricsHandler(state))
	errCh := make(chan error, 2)
	serve(ctx, logger, "operations", operationsServer, errCh)
	serve(ctx, logger, "metrics", metricsServer, errCh)

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case runErr = <-errCh:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	shutdownErr := errors.Join(
		operationsServer.Shutdown(shutdownCtx),
		metricsServer.Shutdown(shutdownCtx),
	)
	return errors.Join(runErr, shutdownErr)
}

func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func serve(ctx context.Context, logger *slog.Logger, name string, server *http.Server, errCh chan<- error) {
	go func() {
		logger.Info("server listening", "server", name, "address", server.Addr)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case errCh <- fmt.Errorf("serve %s: %w", name, err):
			case <-ctx.Done():
			}
		}
	}()
}

func newLogger(levelName string) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(levelName)); err != nil {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
