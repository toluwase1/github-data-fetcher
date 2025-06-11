// internal/github/client_test.go
package github

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v62/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestClient creates a httptest server and a github client pointing to it.
func setupTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	server := httptest.NewServer(handler)

	// We can pass a nil token because we are not authenticating to the real GitHub.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := NewClient("", logger)

	// Override the client's internal http client to point to our test server.
	testClient, err := github.NewClient(server.Client()).WithEnterpriseURLs(server.URL, server.URL)
	require.NoError(t, err)
	client.gh = testClient

	return client, server
}

func TestClient_GetRepository_Retry(t *testing.T) {
	t.Run("succeeds on first try", func(t *testing.T) {
		var requestCount int32
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			assert.Equal(t, "/repos/test/repo", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"id": 1, "name": "repo", "owner": {"login": "test"}}`)
		})
		client, server := setupTestClient(t, handler)
		defer server.Close()

		repo, err := client.GetRepository(context.Background(), "test", "repo")

		require.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
		assert.Equal(t, "repo", repo.Name)
	})

	t.Run("retries on 503 server error and succeeds", func(t *testing.T) {
		var requestCount int32
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&requestCount, 1)
			if count == 1 {
				w.WriteHeader(http.StatusServiceUnavailable) // Fail first time
				return
			}
			w.WriteHeader(http.StatusOK) // Succeed second time
			fmt.Fprintln(w, `{"id": 1, "name": "repo", "owner": {"login": "test"}}`)
		})
		client, server := setupTestClient(t, handler)
		defer server.Close()

		_, err := client.GetRepository(context.Background(), "test", "repo")

		require.NoError(t, err)
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount), "should have made two requests")
	})

	t.Run("handles rate limit error", func(t *testing.T) {
		var requestCount int32
		resetTime := time.Now().Add(50 * time.Millisecond) // Short wait time for test
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&requestCount, 1)
			if count == 1 {
				w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))
				w.WriteHeader(http.StatusForbidden) // RateLimitError is a 403
				fmt.Fprintln(w, `{"message": "API rate limit exceeded"}`)
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"id": 1, "name": "repo", "owner": {"login": "test"}}`)
		})
		client, server := setupTestClient(t, handler)
		defer server.Close()

		startTime := time.Now()
		_, err := client.GetRepository(context.Background(), "test", "repo")
		elapsed := time.Since(startTime)

		require.NoError(t, err)
		assert.True(t, elapsed >= 50*time.Millisecond, "client should wait for rate limit reset")
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	})

	t.Run("fails after max retries on persistent server error", func(t *testing.T) {
		var requestCount int32
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			w.WriteHeader(http.StatusInternalServerError)
		})
		client, server := setupTestClient(t, handler)
		defer server.Close()

		_, err := client.GetRepository(context.Background(), "test", "repo")

		require.Error(t, err)
		var ghErr *github.ErrorResponse
		assert.ErrorAs(t, err, &ghErr)
		assert.Equal(t, http.StatusInternalServerError, ghErr.Response.StatusCode)
		assert.Equal(t, int32(maxRetries), atomic.LoadInt32(&requestCount))
	})
}
