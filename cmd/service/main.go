// cmd/service/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github-data-fetcher/internal/config"
	"github-data-fetcher/internal/github"
	"github-data-fetcher/internal/syncer"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Application startup error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Initialize structured logger
	logLevel := new(slog.LevelVar)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// 2. Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	setLogLevel(cfg.LogLevel, logLevel)
	logger.Info("Configuration loaded successfully")

	// 3. Setup context for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// 4. Initialize database connection and run migrations
	dbpool, err := pgxpool.New(ctx, cfg.DBURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer dbpool.Close()
	logger.Info("Database connection established")

	if err := runMigrations(cfg.DBURL); err != nil {
		return fmt.Errorf("failed to run database migrations: %w", err)
	}
	logger.Info("Database migrations applied successfully")

	// 5. Initialize application components
	ghClient := github.NewClient(cfg.GithubToken, logger)
	appSyncer, err := syncer.NewSyncer(dbpool, ghClient, logger, cfg.ReposToSync, cfg.SyncInterval, cfg.DefaultSyncSinceTime)
	if err != nil {
		return fmt.Errorf("failed to create syncer: %w", err)
	}

	// 6. Start the syncer in a separate goroutine
	go appSyncer.Start(ctx)

	// 7. Wait for shutdown signal
	logger.Info("Application started. Waiting for shutdown signal...")
	<-ctx.Done()
	logger.Info("Shutdown signal received. Exiting.")

	// Allow some time for graceful shutdown of background tasks
	time.Sleep(2 * time.Second)

	return nil
}

func runMigrations(dbURL string) error {
	m, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func setLogLevel(level string, v *slog.LevelVar) {
	switch level {
	case "debug":
		v.Set(slog.LevelDebug)
	case "warn":
		v.Set(slog.LevelWarn)
	case "error":
		v.Set(slog.LevelError)
	default:
		v.Set(slog.LevelInfo)
	}
}
