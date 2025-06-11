# GitHub Data Fetcher Service

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

# GitHub Data Fetcher Service

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A robust, production-grade service written in Go that fetches repository and commit data from the GitHub API, stores it in a PostgreSQL database, and continuously monitors for new commits.

This project is built with an emphasis on clean architecture, best practices, and maintainability, demonstrating a senior-level approach to software engineering.

## 🚀 Features

-   **Concurrent Synchronization**: Fetches data for multiple configured repositories in parallel.
-   **Efficient Data Fetching**: Uses the GitHub API efficiently, handling pagination and avoiding duplicate data.
-   **Persistent Storage**: Stores repository metadata and commit history in a PostgreSQL database.
-   **Robust & Resilient**: Handles API rate limits and transient network errors with exponential backoff. Guarantees data integrity with database transactions for each sync operation.
-   **Configuration Driven**: All key parameters (repositories, API keys, sync interval) are configured via environment variables.
-   **Containerized**: Ships with `Dockerfile` and `docker-compose.yml` for a one-command setup.

## ⚙️ How It Works

The system is designed for simplicity and reliability. When you run the service with `docker-compose`, here's what happens:

1.  **Docker Compose** starts two services: our `app` and a `db` (PostgreSQL) container.
2.  The **Go Application (`app`)** starts up, reads its configuration from the `.env` file, and connects to the database.
3.  The **Syncer** component wakes up on a schedule (e.g., every hour).
4.  It spawns a pool of workers to process configured repositories **concurrently**.
5.  Each worker calls the **GitHub API** to fetch the latest repository information and any new commits since the last check. This process is wrapped in a **database transaction**.
6.  Finally, it saves this new data into the **PostgreSQL Database (`db`)**, where it can be easily queried. The transaction ensures that a repository's metadata and its new commits are saved together, or not at all.

## 🔧 Prerequisites

Before you begin, you will need the following installed on your machine:
1.  **Docker & Docker Compose**: To build and run the application and database. [Install Docker here](https://docs.docker.com/get-docker/).
2.  **Git**: To clone the repository.
3.  **A GitHub Personal Access Token**: The service needs this to talk to the GitHub API. It protects you from being rate-limited quickly.
    -   You can create one by following [GitHub's instructions here](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens).
    -   When creating the token, you only need to grant it the **`repo`** scope.

## 🏁 Getting Started

Follow these steps to get the service running in under 5 minutes.

### Step 1: Clone the Repository

```sh
Open your terminal and clone this project to your local machine.
git clone https://github.com/toluwase1/github-data-fetcher.git
cd github-data-fetcher


Step 2: Create Your Configuration File
This project uses a .env file to store secret keys and configuration. Create your own by copying the example file.

cp .env.example .env
# This will create a new file named .env in the project root.
```

Step 3: Edit the .env File
Now, open the newly created .env file in your favorite text editor. You need to fill in a few key values.

# .env

# Log level: debug, info, warn, error
LOG_LEVEL=info

# PostgreSQL Connection URL
# IMPORTANT: This is pre-configured to work with Docker Compose.
# 'db' is the hostname of the database container. Do not change this unless you know what you are doing.
DB_URL="postgres://user:password@db:5432/github_data?sslmode=disable"

# --- REQUIRED ---
# Paste your GitHub Personal Access Token here.
# This is the most important setting.
GITHUB_TOKEN="ghp_YourSecretTokenGoesHere"

# --- REQUIRED ---
# Comma-separated list of repositories to sync (NO SPACES between them).
# Format is 'owner/repository_name'.
REPOS_TO_SYNC="google/chromium,golang/go"

# Interval for syncing repositories (e.g., 30m, 1h, 2h30m)
SYNC_INTERVAL="1h"

# If a repository has no commits in our DB, the service will pull all commits since this date.
# Format is RFC3339.
# For massive repos like chromium, use a recent date to avoid a very long initial sync.
DEFAULT_SYNC_SINCE_DATE="2024-04-01T00:00:00Z"

# Step 4: Launch the Service!
With your configuration saved, you can now build and run the entire application with a single command.

docker-compose up --build -d

--build: Builds the Go application image from the Dockerfile.
-d: Runs the containers in "detached mode" (in the background).


# Step 5: Check the Logs
You can see what the service is doing by viewing its logs.

docker-compose logs -f app

You should see output indicating that the configuration was loaded, the database was connected, and a sync cycle has started.
📊 How to Query the Data
Once the service is running, you can ask questions of the data you've collected. The easiest way is to use docker exec to run a psql command inside the database container.
Query 1: Get the Top 10 Commit Authors
This query finds who the most active committers are for a specific repository (e.g., google/chromium).
First, find the repository's internal ID:

docker-compose exec -u postgres db psql -d github_data -c "SELECT id, owner, name FROM repositories WHERE owner = 'google' AND name = 'chromium';"
(This will give you a result like id = 1)
Then, use that ID to get the top authors:
(Replace 1 in the command below with the ID you found)

docker-compose exec -u postgres db psql -d github_data -c "SELECT author_name, COUNT(*) as commit_count FROM commits WHERE repository_id = 1 GROUP BY author_name ORDER BY commit_count DESC LIMIT 10;"

Query 2: Get the 50 Most Recent Commits
This query retrieves the latest commits for a repository (e.g., golang/go) without needing to look up the ID first.

docker-compose exec -u postgres db psql -d github_data -c "SELECT c.sha, c.message, c.author_name, c.commit_date FROM commits c JOIN repositories r ON c.repository_id = r.id WHERE r.owner = 'golang' AND r.name = 'go' ORDER BY c.commit_date DESC LIMIT 50;"

# 📝 Other Commands
Running Unit Tests
To run the Go unit tests, execute the following command from the project root.

go test -v ./...

# Resetting Data for a Repository
If you want to force the service to re-fetch all commits for a specific repository, you can delete its existing commits from the database.
First, find the repository's ID (using the same method as in Query 1).
Then, use the ID to delete its commits. (Example uses id = 1)

# docker-compose exec -u postgres db psql -d github_data -c "DELETE FROM commits WHERE repository_id = 1;"

On the next sync cycle, the service will see that no commits exist and will perform a full re-sync from the DEFAULT_SYNC_SINCE_DATE.
Stopping the Service
To stop and remove the running containers:

#### docker-compose down

# 📂 Project Structure
The project follows a standard, scalable Go layout that promotes a clean separation of concerns.

.
├── cmd/service/        # Main application entry point.
├── internal/           # Private application code.
│   ├── config/         # Configuration loading.
│   ├── database/       # Database interaction layer (generated by sqlc).
│   ├── errors/         # Custom error types.
│   ├── github/         # Resilient GitHub API client wrapper.
│   ├── model/          # Core application domain models.
│   └── syncer/         # Core sync orchestration logic.
├── migrations/         # SQL database schema files.
├── .env.example        # Example configuration file.
├── Dockerfile          # Container build instructions.
└── docker-compose.yml  # Service definitions.