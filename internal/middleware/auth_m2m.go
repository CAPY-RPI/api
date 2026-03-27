package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/capyrpi/api/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const BotTokenKey contextKey = "bot_token"

// BotTokenInfo contains information about the authenticated bot
type BotTokenInfo struct {
	TokenID   uuid.UUID
	Name      string
	ExpiresAt *time.Time
}

// M2MAuth middleware validates bot tokens from X-Bot-Token header
func M2MAuth(queries database.Querier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken := r.Header.Get("X-Bot-Token")
			if rawToken == "" {
				http.Error(w, "Missing X-Bot-Token header", http.StatusUnauthorized)
				return
			}

			tokenID, secret, err := ParseBotToken(rawToken)
			if err != nil {
				http.Error(w, "Invalid bot token", http.StatusUnauthorized)
				return
			}

			token, err := queries.GetBotTokenByID(r.Context(), tokenID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					http.Error(w, "Invalid bot token", http.StatusUnauthorized)
					return
				}
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			if !token.IsActive.Bool {
				http.Error(w, "Bot token is inactive", http.StatusUnauthorized)
				return
			}

			if token.ExpiresAt.Valid && token.ExpiresAt.Time.Before(time.Now()) {
				http.Error(w, "Bot token is expired", http.StatusUnauthorized)
				return
			}

			if !ValidateBotToken(secret, token.TokenHash) {
				http.Error(w, "Invalid bot token", http.StatusUnauthorized)
				return
			}

			_ = queries.UpdateBotTokenLastUsed(r.Context(), token.TokenID)

			ctx := context.WithValue(r.Context(), BotTokenKey, &BotTokenInfo{
				TokenID:   token.TokenID,
				Name:      token.Name,
				ExpiresAt: botTokenExpiry(token),
			})
			ctx = context.WithValue(ctx, AuthTypeKey, "bot")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ValidateBotToken validates a raw token against a stored hash
func ValidateBotToken(rawToken, storedHash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(rawToken))
	return err == nil
}

// GetBotToken extracts bot token info from the request context
func GetBotToken(ctx context.Context) (*BotTokenInfo, bool) {
	info, ok := ctx.Value(BotTokenKey).(*BotTokenInfo)
	return info, ok
}

func ParseBotToken(rawToken string) (uuid.UUID, string, error) {
	tokenIDPart, secret, ok := strings.Cut(rawToken, ".")
	if !ok || secret == "" {
		return uuid.Nil, "", errors.New("invalid token format")
	}

	tokenID, err := uuid.Parse(tokenIDPart)
	if err != nil {
		return uuid.Nil, "", err
	}

	return tokenID, secret, nil
}

func botTokenExpiry(token database.BotToken) *time.Time {
	if !token.ExpiresAt.Valid {
		return nil
	}

	expiresAt := token.ExpiresAt.Time
	return &expiresAt
}
