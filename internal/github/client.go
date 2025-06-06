// internal/github/client.go
package github

import (
	"context"
	"database/sql"
	"github-data-fetcher/internal/model"
	"log/slog"
	"time"

	"github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"
)

// Client is a wrapper around the go-github client.
type Client struct {
	gh     *github.Client
	logger *slog.Logger
}

// NewClient creates and configures a new Client instance.
// The provided token is used to create an authenticated http.Client.
func NewClient(token string, logger *slog.Logger) *Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return &Client{
		gh:     github.NewClient(tc),
		logger: logger,
	}
}

// GetRepository fetches repository details and translates them to our internal model.
func (c *Client) GetRepository(ctx context.Context, owner, name string) (*model.Repository, error) {
	repo, _, err := c.gh.Repositories.Get(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	return toInternalRepository(repo), nil
}

// GetCommits fetches all commits for a repository since a given time.
// It handles API pagination transparently.
func (c *Client) GetCommits(ctx context.Context, owner, name string, since time.Time) ([]model.Commit, error) {
	var allCommits []model.Commit

	opts := &github.CommitsListOptions{
		Since: since,
		ListOptions: github.ListOptions{
			PerPage: 100, // Max per page
		},
	}

	for {
		c.logger.Debug("Fetching commits page", "owner", owner, "repo", name, "page", opts.Page)

		commits, resp, err := c.gh.Repositories.ListCommits(ctx, owner, name, opts)
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

// toInternalRepository translates a github.Repository object to our internal model.Repository.
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

// toInternalCommit translates a github.RepositoryCommit object to our internal model.Commit.
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

// toNullString is a helper to convert a pointer-to-string to a sql.NullString.
func toNullString(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{
		String: *s,
		Valid:  true,
	}
}
