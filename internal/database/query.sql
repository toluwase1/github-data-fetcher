-- internal/database/query.sql

-- name: GetRepositoryByOwnerAndName :one
SELECT * FROM repositories
WHERE owner = $1 AND name = $2
LIMIT 1;

-- name: CreateRepository :one
INSERT INTO repositories (
    github_repo_id, owner, name, description, url, language,
    forks_count, stars_count, open_issues_count, watchers_count,
    repo_created_at, repo_updated_at
) VALUES (
             $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
         )
    RETURNING *;

-- name: UpdateRepositorySyncData :one
UPDATE repositories
SET
    description = $2,
    language = $3,
    forks_count = $4,
    stars_count = $5,
    open_issues_count = $6,
    watchers_count = $7,
    repo_updated_at = $8,
    last_synced_at = NOW(),
    updated_at = NOW()
WHERE id = $1
    RETURNING *;


-- name: GetLatestCommitDateForRepo :one
SELECT MAX(commit_date)::timestamp AS max_date FROM commits
WHERE repository_id = $1;

-- name: CreateCommits :copyfrom
INSERT INTO commits (
    sha, repository_id, author_name, author_email, message, url, commit_date
) VALUES (
             $1, $2, $3, $4, $5, $6, $7
         );

-- name: GetTopNCommitAuthors :many
SELECT
    author_name,
    author_email,
    COUNT(*) as commit_count
FROM commits
WHERE repository_id = $1
GROUP BY author_name, author_email
ORDER BY commit_count DESC
LIMIT $2;

-- name: GetCommitsByRepoID :many
SELECT * FROM commits
WHERE repository_id = $1
ORDER BY commit_date DESC;