package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/capyrpi/api/internal/database"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const (
	UserClaimsKey contextKey = "user_claims"
	AuthTypeKey   contextKey = "auth_type"
)

// UserClaims represents the JWT claims for a human user
type UserClaims struct {
	UserID string `json:"uid"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type UserLookup interface {
	GetUserByID(ctx context.Context, uid uuid.UUID) (database.User, error)
}

// Auth middleware validates JWT tokens from cookies or Authorization header
func Auth(jwtSecret string, userLookup UserLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenString string

			// Try cookie first (human users)
			cookie, err := r.Cookie("capy_auth")
			if err == nil {
				tokenString = cookie.Value
			}

			// Fall back to Authorization header
			if tokenString == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					tokenString = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if tokenString == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Parse and validate JWT
			claims := &UserClaims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			uid, err := uuid.Parse(claims.UserID)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			user, err := userLookup.GetUserByID(r.Context(), uid)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			if !user.Role.Valid || string(user.Role.UserRole) != claims.Role {
				http.Error(w, "Stale token", http.StatusUnauthorized)
				return
			}

			// Add claims to context
			ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
			ctx = context.WithValue(ctx, AuthTypeKey, "human")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserClaims extracts user claims from the request context
func GetUserClaims(ctx context.Context) (*UserClaims, bool) {
	claims, ok := ctx.Value(UserClaimsKey).(*UserClaims)
	return claims, ok
}

// GetAuthType returns the authentication type from context ("human" or "bot")
func GetAuthType(ctx context.Context) string {
	authType, ok := ctx.Value(AuthTypeKey).(string)
	if !ok {
		return ""
	}
	return authType
}
