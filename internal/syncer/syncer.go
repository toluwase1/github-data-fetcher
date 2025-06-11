// internal/syncer/syncer.go
package syncer

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github-data-fetcher/internal/database"
	custom_errors "github-data-fetcher/internal/errors"
	"github-data-fetcher/internal/github"
	"github-data-fetcher/internal/model"
)

const (
	// Number of repositories to sync in parallel
	concurrency = 5
)

// RepoIdentifier holds the owner and name of a repository.
type RepoIdentifier struct {
	Owner string
	Name  string
}

// Syncer orchestrates the fetching and storing of data.
type Syncer struct {
	dbpool       *pgxpool.Pool
	ghClient     *github.Client
	logger       *slog.Logger
	reposToSync  []RepoIdentifier
	syncInterval time.Duration
	defaultSince time.Time
}

// NewSyncer creates a new Syncer instance.
func NewSyncer(dbpool *pgxpool.Pool, ghClient *github.Client, logger *slog.Logger, repos []string, interval time.Duration, defaultSince time.Time) (*Syncer, error) {
	parsedRepos, err := parseRepoIdentifiers(repos)
	if err != nil {
		return nil, err
	}

	return &Syncer{
		dbpool:       dbpool,
		ghClient:     ghClient,
		logger:       logger,
		reposToSync:  parsedRepos,
		syncInterval: interval,
		defaultSince: defaultSince,
	}, nil
}

// Start begins the continuous synchronization process.
func (s *Syncer) Start(ctx context.Context) {
	s.logger.Info("Starting syncer", "interval", s.syncInterval.String(), "concurrency", concurrency)
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	s.runSyncCycle(ctx) // Initial sync

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

// runSyncCycle performs a synchronization pass for all configured repositories concurrently.
func (s *Syncer) runSyncCycle(ctx context.Context) {
	s.logger.Info("Starting new sync cycle")
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, repoID := range s.reposToSync {
		repoID := repoID
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}
			err := s.syncRepoInTransaction(gctx, repoID)
			if err != nil && !errors.Is(err, context.Canceled) {
				s.logger.Error("Failed to sync repository", "owner", repoID.Owner, "repo", repoID.Name, "error", err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		s.logger.Error("Sync cycle finished with an error", "error", err)
	} else {
		s.logger.Info("Sync cycle finished")
	}
}

// syncRepoInTransaction wraps the sync logic for a single repo in a DB transaction.
func (s *Syncer) syncRepoInTransaction(ctx context.Context, id RepoIdentifier) error {
	tx, err := s.dbpool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // Rollback is a no-op if the transaction is already committed.

	qtx := database.New(tx)
	err = s.syncRepo(ctx, qtx, id)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// syncRepo handles the full synchronization logic for a single repository.
func (s *Syncer) syncRepo(ctx context.Context, q database.Querier, id RepoIdentifier) error {
	// ** THIS IS THE CORRECTED LINE **
	logger := s.logger.With("owner", id.Owner, "repo", id.Name)
	logger.Info("Syncing repository")

	ghRepo, err := s.ghClient.GetRepository(ctx, id.Owner, id.Name)
	if err != nil {
		return err
	}

	dbRepo, err := s.upsertRepository(ctx, q, ghRepo)
	if err != nil {
		return err
	}
	logger = logger.With("repo_id", dbRepo.ID)

	since, err := s.getSinceTimestamp(ctx, q, dbRepo.ID)
	if err != nil {
		return err
	}
	logger.Info("Fetching commits since", "timestamp", since.Format(time.RFC3339))

	commits, err := s.ghClient.GetCommits(ctx, id.Owner, id.Name, since)
	if err != nil {
		return err
	}

	if len(commits) == 0 {
		logger.Info("No new commits found")
		// Still update repo sync time even if no new commits, and do it inside the transaction.
		_, err := q.UpdateRepositorySyncData(ctx, database.UpdateRepositorySyncDataParams{ID: dbRepo.ID})
		return err
	}

	logger.Info("Found new commits", "count", len(commits))
	n, err := q.CreateCommits(ctx, prepareCommitBulkInsert(dbRepo.ID, commits))
	if err != nil {
		return err
	}
	logger.Info("Successfully inserted commits into database", "count", n)

	return nil
}

// upsertRepository creates or updates a repository.
func (s *Syncer) upsertRepository(ctx context.Context, q database.Querier, repo *model.Repository) (database.Repository, error) {
	existingRepo, err := q.GetRepositoryByOwnerAndName(ctx, database.GetRepositoryByOwnerAndNameParams{
		Owner: repo.Owner,
		Name:  repo.Name,
	})

	if errors.Is(err, pgx.ErrNoRows) {
		s.logger.Info("Repository not found in DB, creating new entry")
		return q.CreateRepository(ctx, database.CreateRepositoryParams{
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
	return q.UpdateRepositorySyncData(ctx, database.UpdateRepositorySyncDataParams{
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

func (s *Syncer) getSinceTimestamp(ctx context.Context, q database.Querier, repoID int64) (time.Time, error) {
	latestCommitDate, err := q.GetLatestCommitDateForRepo(ctx, repoID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, err
	}

	if !latestCommitDate.Valid {
		s.logger.Info("No existing commits found for repository, using default start date", "default_since", s.defaultSince)
		return s.defaultSince, nil
	}

	s.logger.Info("Found latest commit in DB", "timestamp", latestCommitDate.Time)
	return latestCommitDate.Time.Add(1 * time.Second), nil
}

func parseRepoIdentifiers(repos []string) ([]RepoIdentifier, error) {
	var identifiers []RepoIdentifier
	for _, r := range repos {
		parts := strings.Split(r, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, &custom_errors.ErrInvalidRepoFormat{Repo: r}
		}
		identifiers = append(identifiers, RepoIdentifier{Owner: parts[0], Name: parts[1]})
	}
	return identifiers, nil
}

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

func toSQLNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{
		String: *s,
		Valid:  *s != "",
	}
}
