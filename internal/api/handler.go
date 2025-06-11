// internal/api/handler.go
package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"

	"github-data-fetcher/internal/database"
)

// Handler is the container for API dependencies.
type Handler struct {
	db     database.Querier
	logger *slog.Logger
}

// NewRouter creates and configures a new chi router with all API routes.
func NewRouter(db database.Querier, logger *slog.Logger) http.Handler {
	h := &Handler{
		db:     db,
		logger: logger,
	}

	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger) // Chi's default logger
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// API Routes
	r.Get("/health", h.healthCheck)
	r.Route("/v1", func(r chi.Router) {
		r.Get("/repos/{owner}/{name}/commits", h.getCommits)
		r.Get("/repos/{owner}/{name}/stats/top-committers", h.getTopCommitters)
	})

	return r
}

// healthCheck is a simple health endpoint.
func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	respondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// getCommits handles the request to retrieve commits for a repository.
// GET /v1/repos/{owner}/{name}/commits
func (h *Handler) getCommits(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	name := chi.URLParam(r, "name")

	repo, err := h.db.GetRepositoryByOwnerAndName(r.Context(), database.GetRepositoryByOwnerAndNameParams{
		Owner: owner,
		Name:  name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, http.StatusNotFound, "Repository not found")
			return
		}
		h.logger.Error("Failed to get repository", "error", err)
		respondWithError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	commits, err := h.db.GetCommitsByRepoID(r.Context(), repo.ID)
	if err != nil {
		h.logger.Error("Failed to get commits", "error", err)
		respondWithError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	respondWithJSON(w, http.StatusOK, commits)
}

// getTopCommitters handles the request for top commit authors.
// GET /v1/repos/{owner}/{name}/stats/top-committers?limit=N
func (h *Handler) getTopCommitters(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	name := chi.URLParam(r, "name")

	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "10" // Default limit
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		respondWithError(w, http.StatusBadRequest, "Invalid 'limit' parameter. Must be an integer between 1 and 100.")
		return
	}

	repo, err := h.db.GetRepositoryByOwnerAndName(r.Context(), database.GetRepositoryByOwnerAndNameParams{
		Owner: owner,
		Name:  name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, http.StatusNotFound, "Repository not found")
			return
		}
		h.logger.Error("Failed to get repository", "error", err)
		respondWithError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	authors, err := h.db.GetTopNCommitAuthors(r.Context(), database.GetTopNCommitAuthorsParams{
		RepositoryID: repo.ID,
		Limit:        int32(limit),
	})
	if err != nil {
		h.logger.Error("Failed to get top commit authors", "error", err)
		respondWithError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	respondWithJSON(w, http.StatusOK, authors)
}
