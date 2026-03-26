package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

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
}

// New creates a new Handler with the given dependencies
func New(queries database.Querier, cfg *config.Config) *Handler {
	return &Handler{
		queries:       queries,
		Config:        cfg,
		googleAuth:    oauth.NewGoogleProvider(cfg.OAuth.Google.ClientID, cfg.OAuth.Google.ClientSecret, cfg.OAuth.Google.RedirectURL),
		microsoftAuth: oauth.NewMicrosoftProvider(cfg.OAuth.Microsoft.ClientID, cfg.OAuth.Microsoft.ClientSecret, cfg.OAuth.Microsoft.RedirectURL, cfg.OAuth.Microsoft.TenantID),
	}
}

func (h *Handler) isLocalhost(r *http.Request) bool {
	if h.Config.Env != "development" {
		return false
	}
	host := r.Host
	return strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1")
}

func (h *Handler) getCookieDomain(r *http.Request) string {
	if h.isLocalhost(r) {
		return "localhost"
	}
	return h.Config.Cookie.Domain
}

func (h *Handler) getOAuthRedirectURL(r *http.Request, providerRedirectURL string) string {
	if !h.isLocalhost(r) {
		return ""
	}

	// If we're on localhost in dev mode, try to use localhost for the redirect URL
	// We assume the port is the same as the current request
	if strings.Contains(providerRedirectURL, "://") {
		// Replace the host part with localhost:port
		parts := strings.SplitN(providerRedirectURL, "/", 4)
		if len(parts) >= 4 {
			return "http://" + r.Host + "/" + parts[3]
		}
	}
	return ""
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
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
