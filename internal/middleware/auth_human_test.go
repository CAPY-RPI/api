package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type stubUserLookup struct {
	userByID map[uuid.UUID]database.User
}

func (s stubUserLookup) GetUserByID(_ context.Context, uid uuid.UUID) (database.User, error) {
	if user, ok := s.userByID[uid]; ok {
		return user, nil
	}
	return database.User{}, assert.AnError
}

func TestAuth(t *testing.T) {
	secret := "test-secret"
	uid := uuid.New()
	lookup := stubUserLookup{
		userByID: map[uuid.UUID]database.User{
			uid: {
				Uid:  uid,
				Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
			},
		},
	}

	tests := []struct {
		name           string
		tokenSetup     func() *http.Request
		expectedStatus int
	}{
		{
			name: "ValidCookie",
			tokenSetup: func() *http.Request {
				// Generate token
				claims := middleware.UserClaims{
					UserID: uid.String(),
					Role:   string(database.UserRoleStudent),
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenStr, _ := token.SignedString([]byte(secret))

				req := httptest.NewRequest("GET", "/", nil)
				req.AddCookie(&http.Cookie{Name: "capy_auth", Value: tokenStr})
				return req
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "StaleRoleToken",
			tokenSetup: func() *http.Request {
				claims := middleware.UserClaims{
					UserID: uid.String(),
					Role:   string(database.UserRoleDev),
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenStr, _ := token.SignedString([]byte(secret))

				req := httptest.NewRequest("GET", "/", nil)
				req.AddCookie(&http.Cookie{Name: "capy_auth", Value: tokenStr})
				return req
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "MissingToken",
			tokenSetup: func() *http.Request {
				return httptest.NewRequest("GET", "/", nil)
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "InvalidToken",
			tokenSetup: func() *http.Request {
				req := httptest.NewRequest("GET", "/", nil)
				req.AddCookie(&http.Cookie{Name: "capy_auth", Value: "invalid-token"})
				return req
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "ExpiredToken",
			tokenSetup: func() *http.Request {
				claims := middleware.UserClaims{
					UserID: uid.String(),
					Role:   string(database.UserRoleStudent),
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenStr, _ := token.SignedString([]byte(secret))

				req := httptest.NewRequest("GET", "/", nil)
				req.AddCookie(&http.Cookie{Name: "capy_auth", Value: tokenStr})
				return req
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := middleware.Auth(secret, lookup)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// If we get here, check context
				claims, ok := middleware.GetUserClaims(r.Context())
				if ok {
					assert.Equal(t, uid.String(), claims.UserID)
					assert.Equal(t, "human", middleware.GetAuthType(r.Context()))
				}
				w.WriteHeader(http.StatusOK)
			}))

			req := tt.tokenSetup()
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}
