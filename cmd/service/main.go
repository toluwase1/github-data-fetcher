// cmd/service/main.go
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

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github-data-fetcher/internal/api"
	"github-data-fetcher/internal/config"
	"github-data-fetcher/internal/database"
	"github-data-fetcher/internal/github"
	"github-data-fetcher/internal/syncer"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Application shutdown with error", "error", err)
		os.Exit(1)
	}
	slog.Info("Application stopped gracefully")
}

func run() error {
	logLevel := new(slog.LevelVar)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	setLogLevel(cfg.LogLevel, logLevel)
	logger.Info("Configuration loaded successfully")

	// Use an errgroup with a cancellable context to manage all services.
	g, ctx := errgroup.WithContext(context.Background())

	// Listen for OS signals to trigger a graceful shutdown.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	// --- Service 1: The Syncer ---
	g.Go(func() error {
		ghClient := github.NewClient(cfg.GithubToken, logger)
		appSyncer, err := syncer.NewSyncer(dbpool, ghClient, logger, cfg.ReposToSync, cfg.SyncInterval, cfg.DefaultSyncSinceTime)
		if err != nil {
			return fmt.Errorf("failed to create syncer: %w", err)
		}
		appSyncer.Start(ctx)
		logger.Info("Syncer service has stopped.")
		return nil
	})

	// --- Service 2: The API Server ---
	g.Go(func() error {
		dbQuerier := database.New(dbpool)
		router := api.NewRouter(dbQuerier, logger)
		server := &http.Server{
			Addr:         ":8080",
			Handler:      router,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		}

		// This separate goroutine waits for the context to be cancelled and then
		// gracefully shuts down the server.
		go func() {
			<-ctx.Done() // Block until a shutdown signal is received
			logger.Info("Shutdown signal received, shutting down API server...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				logger.Error("API server shutdown error", "error", err)
			}
		}()

		logger.Info("API server starting", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("api server failed to start: %w", err)
		}

		logger.Info("API server has stopped.")
		return nil
	})

	// Wait for all services in the group to finish.
	return g.Wait()
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
