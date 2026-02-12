package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/capyrpi/api/internal/auth/adapters"
	"github.com/capyrpi/api/internal/auth/ports"
	"github.com/capyrpi/api/internal/auth/service"
	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/oauth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Handler holds dependencies for HTTP handlers
type Handler struct {
	queries       database.Querier
	Config        *config.Config
	googleAuth    *oauth.GoogleProvider
	microsoftAuth *oauth.MicrosoftProvider
	authService   *service.AuthService
}

// New creates a new Handler with the given dependencies
func New(queries database.Querier, cfg *config.Config) *Handler {
	googleProvider := oauth.NewGoogleProvider(cfg.OAuth.Google.ClientID, cfg.OAuth.Google.ClientSecret, cfg.OAuth.Google.RedirectURL)
	microsoftProvider := oauth.NewMicrosoftProvider(cfg.OAuth.Microsoft.ClientID, cfg.OAuth.Microsoft.ClientSecret, cfg.OAuth.Microsoft.RedirectURL, cfg.OAuth.Microsoft.TenantID)

	userRepoAdapter := adapters.NewUserRepoAdapter(queries)
	botRepoAdapter, ok := userRepoAdapter.(ports.BotRepo)
	if !ok {
		panic("UserRepoAdapter does not implement BotRepo")
	}

	tokenProviderAdapter := adapters.NewJWTAdapter(cfg)
	googleAdapter := adapters.NewGoogleOAuthAdapter(googleProvider)
	microsoftAdapter := adapters.NewMicrosoftOAuthAdapter(microsoftProvider)

	authService := service.NewAuthService(
		userRepoAdapter,
		botRepoAdapter,
		tokenProviderAdapter,
		googleAdapter,
		microsoftAdapter,
	)

	return &Handler{
		queries:       queries,
		Config:        cfg,
		googleAuth:    googleProvider,
		microsoftAuth: microsoftProvider,
		authService:   authService,
	}
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// respondJSON writes a JSON response with the given status code
func (h *Handler) respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

// respondError writes an error response
func (h *Handler) respondError(w http.ResponseWriter, status int, message string) {
	h.respondJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}

// handleDBError handles database errors and responds appropriately
func (h *Handler) handleDBError(w http.ResponseWriter, err error) {
	if err == pgx.ErrNoRows {
		h.respondError(w, http.StatusNotFound, "Resource not found")
		return
	}

	var pgErr *pgconn.PgError
	if x, ok := err.(*pgconn.PgError); ok {
		pgErr = x
		switch pgErr.Code {
		case "23505": // unique_violation
			h.respondError(w, http.StatusConflict, "Resource already exists")
			return
		case "23503": // foreign_key_violation
			h.respondError(w, http.StatusBadRequest, "Referenced resource not found")
			return
		}
	}

	slog.Error("database error", "error", err)
	h.respondError(w, http.StatusInternalServerError, "Internal server error")
}

// Health returns a simple health check response
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
