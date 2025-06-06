// internal/model/models.go
package model

import (
	"database/sql" // sql.NullTime is still useful for LastSyncedAt
	"time"
)

// Repository represents the metadata of a GitHub repository.
type Repository struct {
	ID              int64
	GithubRepoID    int64 `json:"github_repo_id"`
	Owner           string
	Name            string
	Description     *string
	URL             string
	Language        *string
	ForksCount      int
	StarsCount      int
	OpenIssuesCount int
	WatchersCount   int
	RepoCreatedAt   time.Time
	RepoUpdatedAt   time.Time
	LastSyncedAt    sql.NullTime
	DBCreatedAt     time.Time
	DBUpdatedAt     time.Time
}

type Commit struct {
	SHA          string
	RepositoryID int64
	AuthorName   string
	AuthorEmail  string
	Message      string
	URL          string
	CommitDate   time.Time
	DBCreatedAt  time.Time
}
