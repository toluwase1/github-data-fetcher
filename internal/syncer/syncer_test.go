// internal/syncer/syncer_test.go
package syncer

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github-data-fetcher/internal/database"
	"github-data-fetcher/internal/model"
)

// MockQuerier is a mock of the database.Querier interface.
type MockQuerier struct {
	mock.Mock
}

func (m *MockQuerier) CreateCommits(ctx context.Context, arg []database.CreateCommitsParams) (int64, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(int64), args.Error(1)
}
func (m *MockQuerier) CreateRepository(ctx context.Context, arg database.CreateRepositoryParams) (database.Repository, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(database.Repository), args.Error(1)
}
func (m *MockQuerier) GetCommitsByRepoID(ctx context.Context, repositoryID int64) ([]database.Commit, error) {
	args := m.Called(ctx, repositoryID)
	return args.Get(0).([]database.Commit), args.Error(1)
}
func (m *MockQuerier) GetLatestCommitDateForRepo(ctx context.Context, repositoryID int64) (pgtype.Timestamp, error) {
	args := m.Called(ctx, repositoryID)
	return args.Get(0).(pgtype.Timestamp), args.Error(1)
}
func (m *MockQuerier) GetRepositoryByOwnerAndName(ctx context.Context, arg database.GetRepositoryByOwnerAndNameParams) (database.Repository, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(database.Repository), args.Error(1)
}
func (m *MockQuerier) GetTopNCommitAuthors(ctx context.Context, arg database.GetTopNCommitAuthorsParams) ([]database.GetTopNCommitAuthorsRow, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).([]database.GetTopNCommitAuthorsRow), args.Error(1)
}
func (m *MockQuerier) UpdateRepositorySyncData(ctx context.Context, arg database.UpdateRepositorySyncDataParams) (database.Repository, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(database.Repository), args.Error(1)
}

func TestSyncer_UpsertRepository(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.Background()

	ghRepo := &model.Repository{
		GithubRepoID:  12345,
		Owner:         "test-owner",
		Name:          "test-repo",
		URL:           "http://example.com",
		ForksCount:    10,
		StarsCount:    20,
		RepoUpdatedAt: time.Now(),
	}

	t.Run("creates a new repository if it does not exist", func(t *testing.T) {
		mockQ := new(MockQuerier)
		syncer := &Syncer{logger: logger}

		mockQ.On("GetRepositoryByOwnerAndName", ctx, mock.Anything).Return(database.Repository{}, pgx.ErrNoRows).Once()
		expectedRepo := database.Repository{ID: 1, Owner: "test-owner", Name: "test-repo"}
		mockQ.On("CreateRepository", ctx, mock.Anything).Return(expectedRepo, nil).Once()

		resultRepo, err := syncer.upsertRepository(ctx, mockQ, ghRepo)

		assert.NoError(t, err)
		assert.Equal(t, expectedRepo, resultRepo)
		mockQ.AssertExpectations(t)
	})

	t.Run("updates an existing repository if it is found", func(t *testing.T) {
		mockQ := new(MockQuerier)
		syncer := &Syncer{logger: logger}

		existingRepo := database.Repository{ID: 1, Owner: "test-owner", Name: "test-repo"}
		mockQ.On("GetRepositoryByOwnerAndName", ctx, mock.Anything).Return(existingRepo, nil).Once()

		updatedRepo := database.Repository{ID: 1, Owner: "test-owner", Name: "test-repo", StarsCount: 100}
		mockQ.On("UpdateRepositorySyncData", ctx, mock.Anything).Return(updatedRepo, nil).Once()

		resultRepo, err := syncer.upsertRepository(ctx, mockQ, ghRepo)

		assert.NoError(t, err)
		assert.Equal(t, updatedRepo, resultRepo)
		mockQ.AssertExpectations(t)
		mockQ.AssertNotCalled(t, "CreateRepository")
	})

	t.Run("returns an error if database lookup fails unexpectedly", func(t *testing.T) {
		mockQ := new(MockQuerier)
		syncer := &Syncer{logger: logger}
		dbError := errors.New("unexpected database error")

		mockQ.On("GetRepositoryByOwnerAndName", ctx, mock.Anything).Return(database.Repository{}, dbError).Once()

		_, err := syncer.upsertRepository(ctx, mockQ, ghRepo)

		assert.Error(t, err)
		assert.Equal(t, dbError, err)
		mockQ.AssertExpectations(t)
		mockQ.AssertNotCalled(t, "CreateRepository")
		mockQ.AssertNotCalled(t, "UpdateRepositorySyncData")
	})
}
