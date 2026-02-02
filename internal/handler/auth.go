package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================================
// Auth Response Types
// ============================================================================

type AuthResponse struct {
	User  UserAuthResponse `json:"user"`
	Token string           `json:"token,omitempty"` // Only included for non-cookie auth
}

type UserAuthResponse struct {
	UID       uuid.UUID `json:"uid"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Role      string    `json:"role"`
}

type BotTokenResponse struct {
	TokenID   uuid.UUID  `json:"token_id"`
	Name      string     `json:"name"`
	Token     string     `json:"token,omitempty"` // Only on creation
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	IsActive  bool       `json:"is_active"`
}

type CreateBotTokenRequest struct {
	Name      string     `json:"name" validate:"required,min=1,max=100"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// ============================================================================
// OAuth Handlers (Placeholders - Phase 4.1)
// ============================================================================

// GoogleAuth initiates Google OAuth flow
// @Summary      Initiate Google OAuth
// @Description  Redirects to Google OAuth consent screen
// @Tags         auth
// @Success      302
// @Router       /auth/google [get]
func (h *Handler) GoogleAuth(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement Google OAuth redirect
	h.respondError(w, http.StatusNotImplemented, "Google OAuth not yet implemented")
}

// GoogleCallback handles Google OAuth callback
// @Summary      Google OAuth callback
// @Description  Handles the OAuth callback from Google
// @Tags         auth
// @Param        code   query     string  true  "Authorization code"
// @Param        state  query     string  true  "State token"
// @Success      302
// @Failure      400   {object}  ErrorResponse
// @Router       /auth/google/callback [get]
func (h *Handler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement Google OAuth callback
	h.respondError(w, http.StatusNotImplemented, "Google OAuth not yet implemented")
}

// MicrosoftAuth initiates Microsoft OAuth flow
// @Summary      Initiate Microsoft OAuth
// @Description  Redirects to Microsoft OAuth consent screen
// @Tags         auth
// @Success      302
// @Router       /auth/microsoft [get]
func (h *Handler) MicrosoftAuth(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement Microsoft OAuth redirect
	h.respondError(w, http.StatusNotImplemented, "Microsoft OAuth not yet implemented")
}

// MicrosoftCallback handles Microsoft OAuth callback
// @Summary      Microsoft OAuth callback
// @Description  Handles the OAuth callback from Microsoft
// @Tags         auth
// @Param        code   query     string  true  "Authorization code"
// @Param        state  query     string  true  "State token"
// @Success      302
// @Failure      400   {object}  ErrorResponse
// @Router       /auth/microsoft/callback [get]
func (h *Handler) MicrosoftCallback(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement Microsoft OAuth callback
	h.respondError(w, http.StatusNotImplemented, "Microsoft OAuth not yet implemented")
}

// ============================================================================
// Session Handlers
// ============================================================================

// GetMe returns the current authenticated user
// @Summary      Get current user
// @Description  Returns the currently authenticated user's profile
// @Tags         auth
// @Accept       json
// @Produce      json
// @Success      200   {object}  UserAuthResponse
// @Failure      401   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /auth/me [get]
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetUserClaims(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	uid, err := uuid.Parse(claims.UserID)
	if err != nil {
		h.respondError(w, http.StatusUnauthorized, "Invalid user ID in token")
		return
	}

	user, err := h.queries.GetUserByID(r.Context(), uid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, UserAuthResponse{
		UID:       user.Uid,
		Email:     getEmail(user),
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Role:      string(user.Role.UserRole),
	})
}

// Logout clears the auth cookie
// @Summary      Logout
// @Description  Clears the authentication cookie
// @Tags         auth
// @Success      204
// @Security     CookieAuth
// @Router       /auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "capy_auth",
		Value:    "",
		Path:     "/",
		Domain:   h.config.Cookie.Domain,
		MaxAge:   -1,
		Secure:   h.config.Cookie.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// RefreshToken refreshes the JWT token
// @Summary      Refresh token
// @Description  Issues a new JWT token if the current one is valid
// @Tags         auth
// @Accept       json
// @Produce      json
// @Success      200   {object}  AuthResponse
// @Failure      401   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /auth/refresh [post]
func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetUserClaims(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	uid, err := uuid.Parse(claims.UserID)
	if err != nil {
		h.respondError(w, http.StatusUnauthorized, "Invalid user ID in token")
		return
	}

	user, err := h.queries.GetUserByID(r.Context(), uid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	// Generate new token
	token, err := h.generateJWT(user)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	// Set new cookie
	h.setAuthCookie(w, token)

	h.respondJSON(w, http.StatusOK, AuthResponse{
		User: UserAuthResponse{
			UID:       user.Uid,
			Email:     getEmail(user),
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Role:      string(user.Role.UserRole),
		},
	})
}

// ============================================================================
// Bot Token Management
// ============================================================================

// ListBotTokens lists all bot tokens
// @Summary      List bot tokens
// @Description  Returns all bot tokens (requires faculty role)
// @Tags         bot
// @Accept       json
// @Produce      json
// @Success      200   {array}   BotTokenResponse
// @Failure      403   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /bot/tokens [get]
func (h *Handler) ListBotTokens(w http.ResponseWriter, r *http.Request) {
	// TODO: Check faculty role
	tokens, err := h.queries.ListBotTokens(r.Context())
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	response := make([]BotTokenResponse, len(tokens))
	for i, t := range tokens {
		response[i] = BotTokenResponse{
			TokenID:   t.TokenID,
			Name:      t.Name,
			CreatedAt: t.CreatedAt.Time,
			ExpiresAt: fromPgTimestamp(t.ExpiresAt),
			IsActive:  t.IsActive.Bool,
		}
	}

	h.respondJSON(w, http.StatusOK, response)
}

// CreateBotToken creates a new bot token
// @Summary      Create bot token
// @Description  Creates a new bot token (requires faculty role)
// @Tags         bot
// @Accept       json
// @Produce      json
// @Param        body  body      CreateBotTokenRequest  true  "Token data"
// @Success      201   {object}  BotTokenResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /bot/tokens [post]
func (h *Handler) CreateBotToken(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetUserClaims(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	// TODO: Check faculty role

	var req CreateBotTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		h.respondError(w, http.StatusBadRequest, "Name is required")
		return
	}

	// Generate random token
	rawToken, err := generateSecureToken(32)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	// Hash the token for storage
	hashedToken, err := bcrypt.GenerateFromPassword([]byte(rawToken), bcrypt.DefaultCost)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to hash token")
		return
	}

	uid, _ := uuid.Parse(claims.UserID)

	token, err := h.queries.CreateBotToken(r.Context(), database.CreateBotTokenParams{
		TokenHash: string(hashedToken),
		Name:      req.Name,
		CreatedBy: uid,
		ExpiresAt: toPgTimestamp(req.ExpiresAt),
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusCreated, BotTokenResponse{
		TokenID:   token.TokenID,
		Name:      token.Name,
		Token:     rawToken, // Only returned on creation!
		CreatedAt: token.CreatedAt.Time,
		ExpiresAt: fromPgTimestamp(token.ExpiresAt),
		IsActive:  token.IsActive.Bool,
	})
}

// RevokeBotToken revokes a bot token
// @Summary      Revoke bot token
// @Description  Revokes a bot token (requires faculty role)
// @Tags         bot
// @Accept       json
// @Produce      json
// @Param        token_id  path      string  true  "Token UUID"
// @Success      204
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /bot/tokens/{token_id} [delete]
func (h *Handler) RevokeBotToken(w http.ResponseWriter, r *http.Request) {
	tokenIDStr := chi.URLParam(r, "token_id")
	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid token ID")
		return
	}

	// TODO: Check faculty role

	if err := h.queries.RevokeBotToken(r.Context(), tokenID); err != nil {
		h.handleDBError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetBotMe returns info about the current bot token
// @Summary      Get bot info
// @Description  Returns information about the current bot token
// @Tags         bot
// @Accept       json
// @Produce      json
// @Success      200   {object}  BotTokenResponse
// @Failure      401   {object}  ErrorResponse
// @Security     BotToken
// @Router       /bot/me [get]
func (h *Handler) GetBotMe(w http.ResponseWriter, r *http.Request) {
	// Token info would be in context from M2M middleware
	h.respondJSON(w, http.StatusOK, map[string]string{
		"status": "authenticated",
		"type":   "bot",
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

func (h *Handler) generateJWT(user database.User) (string, error) {
	claims := &middleware.UserClaims{
		UserID: user.Uid.String(),
		Email:  getEmail(user),
		Role:   string(user.Role.UserRole),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(h.config.JWT.ExpiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "capy-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.config.JWT.Secret))
}

func (h *Handler) setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "capy_auth",
		Value:    token,
		Path:     "/",
		Domain:   h.config.Cookie.Domain,
		MaxAge:   h.config.JWT.ExpiryHours * 3600,
		Secure:   h.config.Cookie.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func getEmail(user database.User) string {
	if user.SchoolEmail.Valid {
		return user.SchoolEmail.String
	}
	if user.PersonalEmail.Valid {
		return user.PersonalEmail.String
	}
	return ""
}

func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func toPgTimestampFromTime(t time.Time) pgtype.Timestamp {
	return pgtype.Timestamp{Time: t, Valid: true}
}
