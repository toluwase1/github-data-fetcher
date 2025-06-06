// internal/syncer/syncer.go
package syncer

import (
	"context"
	"database/sql"
	"errors"
	"github-data-fetcher/internal/model"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github-data-fetcher/internal/database"
	"github-data-fetcher/internal/github"
)

// RepoIdentifier holds the owner and name of a repository.
type RepoIdentifier struct {
	Owner string
	Name  string
}

// Syncer orchestrates the fetching of data from the GitHub API and storing it in the database.
type Syncer struct {
	db           database.Querier
	ghClient     *github.Client
	logger       *slog.Logger
	reposToSync  []RepoIdentifier
	syncInterval time.Duration
	defaultSince time.Time
}

// NewSyncer creates a new Syncer instance.
func NewSyncer(db database.Querier, ghClient *github.Client, logger *slog.Logger, repos []string, interval time.Duration, defaultSince time.Time) (*Syncer, error) {
	parsedRepos, err := parseRepoIdentifiers(repos)
	if err != nil {
		return nil, err
	}

	return &Syncer{
		db:           db,
		ghClient:     ghClient,
		logger:       logger,
		reposToSync:  parsedRepos,
		syncInterval: interval,
		defaultSince: defaultSince,
	}, nil
}

// Start begins the continuous synchronization process.
// It performs an initial sync immediately and then ticks at the configured interval.
// This method blocks until the context is canceled.
func (s *Syncer) Start(ctx context.Context) {
	s.logger.Info("Starting syncer", "interval", s.syncInterval.String())
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	// Perform an initial sync immediately on startup.
	s.runSyncCycle(ctx)

	for {
		select {
		case <-ticker.C:
			s.runSyncCycle(ctx)
		case <-ctx.Done():
			s.logger.Info("Syncer shutting down", "reason", ctx.Err())
			return
		}
	}
}

// runSyncCycle performs a single synchronization pass for all configured repositories.
func (s *Syncer) runSyncCycle(ctx context.Context) {
	s.logger.Info("Starting new sync cycle")
	for _, repoIdentifier := range s.reposToSync {
		if err := ctx.Err(); err != nil {
			s.logger.Warn("Sync cycle aborted due to context cancellation", "repo", repoIdentifier)
			break
		}
		if err := s.syncRepo(ctx, repoIdentifier); err != nil {
			s.logger.Error("Failed to sync repository", "repo", repoIdentifier, "error", err)
		}
	}
	s.logger.Info("Sync cycle finished")
}

// syncRepo handles the full synchronization logic for a single repository.
func (s *Syncer) syncRepo(ctx context.Context, id RepoIdentifier) error {
	logger := s.logger.With("owner", id.Owner, "repo", id.Name)
	logger.Info("Syncing repository")

	// 1. Fetch latest repo metadata from GitHub.
	ghRepo, err := s.ghClient.GetRepository(ctx, id.Owner, id.Name)
	if err != nil {
		return err
	}

	// 2. Upsert repository data into our DB. This ensures the repo record is present and up-to-date.
	dbRepo, err := s.upsertRepository(ctx, ghRepo)
	if err != nil {
		return err
	}
	logger = logger.With("repo_id", dbRepo.ID)

	// 3. Determine the timestamp from which to fetch commits.
	since, err := s.getSinceTimestamp(ctx, dbRepo.ID)
	if err != nil {
		return err
	}
	logger.Info("Fetching commits since", "timestamp", since.Format(time.RFC3339))

	// 4. Fetch new commits from GitHub.
	commits, err := s.ghClient.GetCommits(ctx, id.Owner, id.Name, since)
	if err != nil {
		return err
	}

	if len(commits) == 0 {
		logger.Info("No new commits found")
		// Still update the repo's last_synced_at time
		_, err := s.db.UpdateRepositorySyncData(ctx, database.UpdateRepositorySyncDataParams{ID: dbRepo.ID})
		return err
	}

	// 5. Bulk insert new commits into the database.
	logger.Info("Found new commits", "count", len(commits))
	n, err := s.db.CreateCommits(ctx, prepareCommitBulkInsert(dbRepo.ID, commits))
	if err != nil {
		return err
	}
	logger.Info("Successfully inserted commits into database", "count", n)

	return nil
}

// upsertRepository creates a repository if it doesn't exist, or updates it if it does.
func (s *Syncer) upsertRepository(ctx context.Context, repo *model.Repository) (database.Repository, error) {
	existingRepo, err := s.db.GetRepositoryByOwnerAndName(ctx, database.GetRepositoryByOwnerAndNameParams{
		Owner: repo.Owner,
		Name:  repo.Name,
	})

	if errors.Is(err, pgx.ErrNoRows) {
		s.logger.Info("Repository not found in DB, creating new entry")
		return s.db.CreateRepository(ctx, database.CreateRepositoryParams{
			GithubRepoID:    repo.GithubRepoID,
			Owner:           repo.Owner,
			Name:            repo.Name,
			Description:     toSQLNullString(repo.Description).String,
			Url:             repo.URL,
			Language:        toSQLNullString(repo.Language).String,
			ForksCount:      int32(repo.ForksCount),
			StarsCount:      int32(repo.StarsCount),
			OpenIssuesCount: int32(repo.OpenIssuesCount),
			WatchersCount:   int32(repo.WatchersCount),
			RepoCreatedAt:   repo.RepoCreatedAt,
			RepoUpdatedAt:   repo.RepoUpdatedAt,
		})
	} else if err != nil {
		return database.Repository{}, err
	}

	s.logger.Info("Repository found in DB, updating metadata")
	return s.db.UpdateRepositorySyncData(ctx, database.UpdateRepositorySyncDataParams{
		ID:              existingRepo.ID,
		Description:     toSQLNullString(repo.Description).String,
		Language:        toSQLNullString(repo.Language).String,
		ForksCount:      int32(repo.ForksCount),
		StarsCount:      int32(repo.StarsCount),
		OpenIssuesCount: int32(repo.OpenIssuesCount),
		WatchersCount:   int32(repo.WatchersCount),
		RepoUpdatedAt:   repo.RepoUpdatedAt,
	})
}

// toSQLNullString is a helper to convert a pointer-to-string to a sql.NullString.
func toSQLNullString(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{
		String: *s,
		Valid:  true,
	}
}

// getSinceTimestamp determines the correct time to start fetching commits from.
// It returns the date of the latest commit we have, or the default start date if none exist.
func (s *Syncer) getSinceTimestamp(ctx context.Context, repoID int64) (time.Time, error) {
	latestCommitDate, err := s.db.GetLatestCommitDateForRepo(ctx, repoID)
	if err != nil {
		return time.Time{}, err
	}

	if !latestCommitDate.Valid {
		s.logger.Info("No existing commits found for repository, using default start date", "default_since", s.defaultSince)
		return s.defaultSince, nil
	}

	s.logger.Info("Found latest commit in DB", "timestamp", latestCommitDate.Time)

	return latestCommitDate.Time.Add(1 * time.Second), nil
}

// prepareCommitBulkInsert transforms a slice of our domain model commits into the format
// required by sqlc for a bulk `COPY FROM` operation.
func prepareCommitBulkInsert(repoID int64, commits []model.Commit) []database.CreateCommitsParams {
	params := make([]database.CreateCommitsParams, len(commits))
	for i, c := range commits {
		params[i] = database.CreateCommitsParams{
			RepositoryID: repoID,
			Sha:          c.SHA,
			AuthorName:   c.AuthorName,
			AuthorEmail:  c.AuthorEmail,
			Message:      c.Message,
			Url:          c.URL,
			CommitDate:   c.CommitDate,
		}
	}
	return params
}

func parseRepoIdentifiers(repos []string) ([]RepoIdentifier, error) {
	var identifiers []RepoIdentifier
	for _, r := range repos {
		parts := strings.Split(r, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, errors.New("invalid repository format: " + r + ", expected 'owner/name'")
		}
		identifiers = append(identifiers, RepoIdentifier{Owner: parts[0], Name: parts[1]})
	}
	return identifiers, nil
}
