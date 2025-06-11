//go:build integration

// cmd/service/integration_test.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github-data-fetcher/internal/database"
	"github-data-fetcher/internal/github"
	"github-data-fetcher/internal/syncer"
)

func setupTestDatabase(ctx context.Context, t *testing.T) (*pgxpool.Pool, func()) {
	// Start a postgres container
	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		postgres.WithDatabase("test-db"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second),
		),
	)
	require.NoError(t, err)

	// Get the connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Run migrations
	m, err := migrate.New("file://../../migrations", connStr)
	require.NoError(t, err)
	err = m.Up()
	require.NoError(t, err)

	// Create a connection pool
	dbpool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)

	// Teardown function to be called by the test
	teardown := func() {
		dbpool.Close()
		err := pgContainer.Terminate(ctx)
		require.NoError(t, err)
	}

	return dbpool, teardown
}

func TestSyncer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	dbpool, teardown := setupTestDatabase(ctx, t)
	defer teardown()

	// Setup a mock GitHub API server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/test-owner/test-repo":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id": 123, "owner": {"login": "test-owner"}, "name": "test-repo"}`))
		case "/repos/test-owner/test-repo/commits":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"sha": "abc", "commit": {"author": {"name": "tester", "email": "t@t.com", "date": "2024-01-01T12:00:00Z"}, "message": "feat: new feature"}, "html_url": "url1"},
				{"sha": "def", "commit": {"author": {"name": "tester", "email": "t@t.com", "date": "2024-01-02T12:00:00Z"}, "message": "fix: a bug"}, "html_url": "url2"}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create a github client pointing to the mock server
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ghClient := github.NewClient("", logger)
	ghClient.OverrideBaseURL(server.URL) // Simplified for test; real one is more complex

	// Create the syncer with the REAL database pool and mock GitHub client
	appSyncer, err := syncer.NewSyncer(dbpool, ghClient, logger, []string{"test-owner/test-repo"}, time.Hour, time.Time{})
	require.NoError(t, err)

	// --- ACT ---
	// Run a single sync cycle. We call the internal method directly for this test.
	err = appSyncer.SyncRepoInTransaction(ctx, syncer.RepoIdentifier{Owner: "test-owner", Name: "test-repo"})
	require.NoError(t, err)

	// --- ASSERT ---
	// Query the database directly to verify the data was inserted correctly.
	dbQuerier := database.New(dbpool)
	repo, err := dbQuerier.GetRepositoryByOwnerAndName(ctx, database.GetRepositoryByOwnerAndNameParams{Owner: "test-owner", Name: "test-repo"})
	require.NoError(t, err)
	assert.Equal(t, int64(123), repo.GithubRepoID)
	assert.Equal(t, "test-repo", repo.Name)

	commits, err := dbQuerier.GetCommitsByRepoID(ctx, repo.ID)
	require.NoError(t, err)
	assert.Len(t, commits, 2)
	assert.Equal(t, "abc", commits[1].Sha) // Order is by date DESC
	assert.Equal(t, "def", commits[0].Sha)
	assert.Equal(t, "fix: a bug", commits[0].Message)
}

// Helper to swap base URL on our custom client wrapper for testing
func (c *github.Client) OverrideBaseURL(url string) {
	ghc, _ := ghc.NewClient(nil).WithEnterpriseURLs(url, url)
	c.gh = ghc
}
