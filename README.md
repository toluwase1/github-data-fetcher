# GitHub Data Fetcher Service

A robust, production-grade service written in Go that fetches repository and commit data from the GitHub API, stores it in a PostgreSQL database, and continuously monitors for new commits.

This project is built with an emphasis on clean architecture, best practices, and maintainability, demonstrating a senior-level approach to software engineering.

## Features

-   **Continuous Synchronization**: Fetches data for configured repositories at a regular interval.
-   **Efficient Data Fetching**: Uses the GitHub API efficiently, handling pagination and avoiding duplicate data.
-   **Persistent Storage**: Stores repository metadata and commit history in a PostgreSQL database.
-   **Robust & Resilient**: Handles API errors and guarantees data integrity with database constraints.
-   **Configuration Driven**: All key parameters (repositories, API keys, sync interval) are configured via environment variables.
-   **Containerized**: Ships with `Dockerfile` and `docker-compose.yml` for a one-command setup and reproducible builds.

## Prerequisites

-   Docker & Docker Compose
-   Go 1.22+ (for running tests or local development outside of Docker)
-   A [GitHub Personal Access Token](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens) with `repo` scope.

## Getting Started

1.  **Clone the repository:**
    ```sh
    git clone https://your-repo-url/github-data-fetcher.git
    cd github-data-fetcher
    ```

2.  **Create your environment file:**
    Copy the example file.
    ```sh
    cp .env.example .env
    ```

3.  **Edit the `.env` file:**
    Open the `.env` file and fill in the required values, most importantly `GITHUB_TOKEN` and the `REPOS_TO_SYNC`. For a large repository like Chromium, adjust the `DEFAULT_SYNC_SINCE_DATE` to a more recent date to reduce initial fetch time.
    ```
    DB_URL="postgres://user:password@db:5432/github_data?sslmode=disable"
    GITHUB_TOKEN="ghp_YourSecretTokenGoesHere"
    REPOS_TO_SYNC="google/chromium,golang/go"
    DEFAULT_SYNC_SINCE_DATE="2024-04-01T00:00:00Z" # Example: sync from April 2024
    ```
    **Note:** The `DB_URL` is pre-configured to work with Docker Compose. `db` is the hostname of the database service.

4.  **Launch the service:**
    Build and run the application and database containers in detached mode.
    ```sh
    docker-compose up --build -d
    ```

You can view the service logs with `docker-compose logs -f app`.

## How to Query the Data

Once the service is running and has had time to sync data, you can query the PostgreSQL database directly. You can use your preferred SQL client connected to `localhost:5432` or use `docker exec` for quick queries.

**Example using `docker exec`:**
```sh
docker-compose exec -u postgres db psql -d github_data -c "YOUR_SQL_QUERY_HERE"