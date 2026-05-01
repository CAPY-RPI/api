package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/capyrpi/api/internal/oauth"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

type BotMeResponse struct {
	TokenID   uuid.UUID  `json:"token_id"`
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	AuthType  string     `json:"auth_type"`
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
	state, err := oauth.GenerateStateToken()
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to generate state")
		return
	}

	// Set state cookie to verify callback
	h.setStateCookie(w, r, state)

	redirectURL := h.getOAuthRedirectURL(r, h.Config.OAuth.Google.RedirectURL)
	http.Redirect(w, r, h.googleAuth.GetAuthURL(state, redirectURL), http.StatusFound)
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
	// Verify state
	state := r.URL.Query().Get("state")
	if !h.verifyStateCookie(w, r, state) {
		h.respondError(w, http.StatusBadRequest, "Invalid state parameter")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.respondError(w, http.StatusBadRequest, "Missing auth code")
		return
	}

	redirectURL := h.getOAuthRedirectURL(r, h.Config.OAuth.Google.RedirectURL)
	userInfo, err := h.googleAuth.ExchangeCode(r.Context(), code, redirectURL)
	if err != nil {
		slog.Error("oauth exchange failed", "err", err, "redirect_uri", redirectURL, "host", r.Host, "xfh", r.Header.Get("X-Forwarded-Host"), "xfp", r.Header.Get("X-Forwarded-Proto"))
		h.respondError(w, http.StatusInternalServerError, "Failed to exchange code")
		return
	}

	user, err := h.upsertUser(r.Context(), userInfo.Email, userInfo.GivenName, userInfo.FamilyName)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	token, err := h.generateJWT(user)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to generate session")
		return
	}

	h.setAuthCookie(w, r, token)
	h.respondWithCloseWindow(w)
}

// MicrosoftAuth initiates Microsoft OAuth flow
// @Summary      Initiate Microsoft OAuth
// @Description  Redirects to Microsoft OAuth consent screen
// @Tags         auth
// @Success      302
// @Router       /auth/microsoft [get]
func (h *Handler) MicrosoftAuth(w http.ResponseWriter, r *http.Request) {
	state, err := oauth.GenerateStateToken()
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to generate state")
		return
	}

	h.setStateCookie(w, r, state)
	redirectURL := h.getOAuthRedirectURL(r, h.Config.OAuth.Microsoft.RedirectURL)
	http.Redirect(w, r, h.microsoftAuth.GetAuthURL(state, redirectURL), http.StatusFound)
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
	state := r.URL.Query().Get("state")
	if !h.verifyStateCookie(w, r, state) {
		h.respondError(w, http.StatusBadRequest, "Invalid state parameter")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.respondError(w, http.StatusBadRequest, "Missing auth code")
		return
	}

	redirectURL := h.getOAuthRedirectURL(r, h.Config.OAuth.Microsoft.RedirectURL)
	userInfo, err := h.microsoftAuth.ExchangeCode(r.Context(), code, redirectURL)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to exchange code")
		return
	}

	// Use PrincipalName (email) or Mail
	email := userInfo.UserPrincipalName
	if email == "" {
		email = userInfo.Mail
	}

	user, err := h.upsertUser(r.Context(), email, userInfo.GivenName, userInfo.Surname)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	token, err := h.generateJWT(user)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to generate session")
		return
	}

	h.setAuthCookie(w, r, token)
	h.respondWithCloseWindow(w)
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
		Domain:   h.getCookieDomain(r),
		MaxAge:   -1,
		Secure:   h.Config.Cookie.Secure,
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
	h.setAuthCookie(w, r, token)

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
// @Description  Returns all bot tokens (requires dev role)
// @Tags         bot
// @Accept       json
// @Produce      json
// @Success      200   {array}   BotTokenResponse
// @Failure      403   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /bot/tokens [get]
func (h *Handler) ListBotTokens(w http.ResponseWriter, r *http.Request) {
	if !h.requireDev(w, r) {
		return
	}

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
// @Description  Creates a new bot token (requires dev role). The raw token is returned only once and must be stored by the caller.
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
	claims, ok := h.requireDevClaims(w, r)
	if !ok {
		return
	}

	var req CreateBotTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		h.respondError(w, http.StatusBadRequest, "Name is required")
		return
	}

	secret, err := generateSecureToken(32)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	hashedToken, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
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

	rawToken := formatBotToken(token.TokenID, secret)

	h.respondJSON(w, http.StatusCreated, BotTokenResponse{
		TokenID:   token.TokenID,
		Name:      token.Name,
		Token:     rawToken, // Only returned on creation
		CreatedAt: token.CreatedAt.Time,
		ExpiresAt: fromPgTimestamp(token.ExpiresAt),
		IsActive:  token.IsActive.Bool,
	})
}

// RevokeBotToken revokes a bot token
// @Summary      Revoke bot token
// @Description  Revokes a bot token (requires dev role)
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
	if !h.requireDev(w, r) {
		return
	}

	tokenIDStr := chi.URLParam(r, "token_id")
	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid token ID")
		return
	}

	if err := h.queries.RevokeBotToken(r.Context(), tokenID); err != nil {
		h.handleDBError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetBotMe returns info about the current bot token
// @Summary      Get bot info
// @Description  Returns information about the current bot token. Authenticate with X-Bot-Token: <token_id>.<secret>, for example: curl -H 'X-Bot-Token: <token>' http://localhost:8080/api/v1/bot/me
// @Tags         bot
// @Accept       json
// @Produce      json
// @Success      200   {object}  BotMeResponse
// @Failure      401   {object}  ErrorResponse
// @Security     BotToken
// @Router       /bot/me [get]
func (h *Handler) GetBotMe(w http.ResponseWriter, r *http.Request) {
	token, ok := middleware.GetBotToken(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	h.respondJSON(w, http.StatusOK, BotMeResponse{
		TokenID:   token.TokenID,
		Name:      token.Name,
		ExpiresAt: token.ExpiresAt,
		AuthType:  middleware.GetAuthType(r.Context()),
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

func (h *Handler) generateJWT(user database.User) (string, error) {
	claims := &middleware.UserClaims{
		UserID: user.Uid.String(),
		Email:  getEmail(user),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(h.Config.JWT.ExpiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "capy-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.Config.JWT.Secret))
}

func (h *Handler) setAuthCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "capy_auth",
		Value:    token,
		Path:     "/",
		Domain:   h.getCookieDomain(r),
		MaxAge:   h.Config.JWT.ExpiryHours * 3600,
		Secure:   h.Config.Cookie.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) respondWithCloseWindow(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
    <title>Login Successful</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            height: 100vh;
            margin: 0;
            background-color: #f4f7f6;
            color: #333;
        }
        .container {
            text-align: center;
            padding: 2rem;
            background: white;
            border-radius: 12px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }
        h1 { color: #2ecc71; margin-bottom: 1rem; }
        p { margin-bottom: 2rem; color: #666; }
        .close-hint { font-size: 0.875rem; color: #999; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Login Successful!</h1>
        <p>You have been successfully authenticated.</p>
        <p class="close-hint">This window should close automatically. If not, you can safely close it now.</p>
    </div>
    <script>
        // Attempt to close the window
        window.close();
        
        // Fallback for some browsers if window.close() is blocked
        setTimeout(function() {
            window.close();
        }, 1000);
    </script>
</body>
</html>
`))
}

func (h *Handler) setStateCookie(w http.ResponseWriter, r *http.Request, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/api/v1/auth",
		Domain:   h.getCookieDomain(r),
		MaxAge:   300, // 5 minutes
		Secure:   h.Config.Cookie.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) verifyStateCookie(w http.ResponseWriter, r *http.Request, state string) bool {
	cookie, err := r.Cookie("oauth_state")
	if err != nil {
		return false
	}
	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/api/v1/auth",
		Domain:   h.getCookieDomain(r),
		MaxAge:   -1,
		Secure:   h.Config.Cookie.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return cookie.Value == state
}

func (h *Handler) upsertUser(ctx context.Context, email, firstName, lastName string) (database.User, error) {
	normalizedEmail := normalizeEmail(email)
	pgEmail := toPgTextFromString(normalizedEmail)

	// Check if user exists
	user, err := h.queries.GetUserByEmail(ctx, pgEmail)
	if err == nil {
		return user, nil
	}

	if err != pgx.ErrNoRows {
		return database.User{}, err
	}

	// Create new user
	return h.queries.CreateUser(ctx, database.CreateUserParams{
		FirstName:     firstName,
		LastName:      lastName,
		PersonalEmail: pgEmail, // Default to personal email for oauth
		SchoolEmail:   pgtype.Text{Valid: false},
		Role:          database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
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

func formatBotToken(tokenID uuid.UUID, secret string) string {
	return tokenID.String() + "." + secret
}
