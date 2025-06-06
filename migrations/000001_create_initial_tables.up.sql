-- migrations/000001_create_initial_tables.up.sql
CREATE TABLE repositories (
                              id BIGSERIAL PRIMARY KEY,
                              github_repo_id BIGINT NOT NULL UNIQUE,
                              owner TEXT NOT NULL,
                              name TEXT NOT NULL,
                              description TEXT,
                              url TEXT NOT NULL,
                              language TEXT,
                              forks_count INT NOT NULL DEFAULT 0,
                              stars_count INT NOT NULL DEFAULT 0,
                              open_issues_count INT NOT NULL DEFAULT 0,
                              watchers_count INT NOT NULL DEFAULT 0,
                              repo_created_at TIMESTAMPTZ NOT NULL,
                              repo_updated_at TIMESTAMPTZ NOT NULL,
                              last_synced_at TIMESTAMPTZ,
                              created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                              updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                              CONSTRAINT uq_owner_name UNIQUE (owner, name)
);

CREATE TABLE commits (
                         sha VARCHAR(40) NOT NULL,
                         repository_id BIGINT NOT NULL,
                         author_name TEXT NOT NULL,
                         author_email TEXT NOT NULL,
                         message TEXT NOT NULL,
                         url TEXT NOT NULL,
                         commit_date TIMESTAMPTZ NOT NULL,
                         created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                         PRIMARY KEY (repository_id, sha),
                         CONSTRAINT fk_repository
                             FOREIGN KEY (repository_id)
                                 REFERENCES repositories(id)
                                 ON DELETE CASCADE
);

CREATE INDEX idx_commits_commit_date ON commits(commit_date DESC);
CREATE INDEX idx_commits_author_name ON commits(author_name);