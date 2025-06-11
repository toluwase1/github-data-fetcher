// internal/github/client.go
package github

import (
	"context"
	"errors"
	"github-data-fetcher/internal/model"
	"log/slog"
	"math"
	"math/rand" // Import math/rand
	"time"

	"github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"
)

const (
	maxRetries    = 5
	retryMinDelay = 1 * time.Second
	retryMaxDelay = 120 * time.Second
)

// Client is a wrapper around the go-github client that adds resilience.
type Client struct {
	gh     *github.Client
	logger *slog.Logger
	r      *rand.Rand // Add a random source for jitter
}

// NewClient creates and configures a new Client instance.
func NewClient(token string, logger *slog.Logger) *Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return &Client{
		gh:     github.NewClient(tc),
		logger: logger,
		// Create a non-global random source to be concurrency-safe.
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GetRepository fetches repository details with retry logic.
func (c *Client) GetRepository(ctx context.Context, owner, name string) (*model.Repository, error) {
	var repo *github.Repository
	var resp *github.Response
	var err error

	err = c.retry(ctx, func() (*github.Response, error) {
		repo, resp, err = c.gh.Repositories.Get(ctx, owner, name)
		return resp, err
	})

	if err != nil {
		return nil, err
	}
	return toInternalRepository(repo), nil
}

// GetCommits fetches all commits for a repository since a given time, with retries and pagination.
func (c *Client) GetCommits(ctx context.Context, owner, name string, since time.Time) ([]model.Commit, error) {
	var allCommits []model.Commit

	opts := &github.CommitsListOptions{
		Since: since,
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	for {
		var commits []*github.RepositoryCommit
		var resp *github.Response
		var err error

		err = c.retry(ctx, func() (*github.Response, error) {
			c.logger.Debug("Fetching commits page", "owner", owner, "repo", name, "page", opts.Page)
			commits, resp, err = c.gh.Repositories.ListCommits(ctx, owner, name, opts)
			return resp, err
		})
		if err != nil {
			return nil, err
		}

		for _, commit := range commits {
			allCommits = append(allCommits, toInternalCommit(commit))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allCommits, nil
}

// retry is a generic retry wrapper for GitHub API calls.
func (c *Client) retry(ctx context.Context, fn func() (*github.Response, error)) error {
	var err error
	var resp *github.Response

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err = fn()
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		var rateLimitErr *github.RateLimitError
		if errors.As(err, &rateLimitErr) {
			sleepDuration := time.Until(rateLimitErr.Rate.Reset.Time)
			c.logger.Warn("GitHub API rate limit exceeded. Waiting for reset.", "wait_duration", sleepDuration)
			select {
			case <-time.After(sleepDuration):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		var abuseErr *github.AbuseRateLimitError
		if errors.As(err, &abuseErr) {
			sleepDuration := abuseErr.GetRetryAfter()
			c.logger.Warn("GitHub API abuse mechanism triggered. Waiting.", "wait_duration", sleepDuration)
			select {
			case <-time.After(sleepDuration):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if resp != nil && resp.StatusCode >= 500 {
			backoff := float64(retryMinDelay) * math.Pow(2, float64(attempt))
			if backoff > float64(retryMaxDelay) {
				backoff = float64(retryMaxDelay)
			}
			// Add jitter: backoff ± 25%
			jitter := (c.r.Float64() - 0.5) * backoff * 0.5
			sleepDuration := time.Duration(backoff + jitter)

			c.logger.Warn("GitHub API server error. Retrying.", "status_code", resp.StatusCode, "attempt", attempt+1, "backoff", sleepDuration)
			select {
			case <-time.After(sleepDuration):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return err
	}

	return err
}

func toInternalRepository(r *github.Repository) *model.Repository {
	return &model.Repository{
		GithubRepoID:    r.GetID(),
		Owner:           r.GetOwner().GetLogin(),
		Name:            r.GetName(),
		Description:     r.Description,
		URL:             r.GetHTMLURL(),
		Language:        r.Language,
		ForksCount:      r.GetForksCount(),
		StarsCount:      r.GetStargazersCount(),
		OpenIssuesCount: r.GetOpenIssuesCount(),
		WatchersCount:   r.GetWatchersCount(),
		RepoCreatedAt:   r.GetCreatedAt().Time,
		RepoUpdatedAt:   r.GetUpdatedAt().Time,
	}
}

func toInternalCommit(c *github.RepositoryCommit) model.Commit {
	return model.Commit{
		SHA:         c.GetSHA(),
		AuthorName:  c.GetCommit().GetAuthor().GetName(),
		AuthorEmail: c.GetCommit().GetAuthor().GetEmail(),
		Message:     c.GetCommit().GetMessage(),
		URL:         c.GetHTMLURL(),
		CommitDate:  c.GetCommit().GetAuthor().GetDate().Time,
	}
}
