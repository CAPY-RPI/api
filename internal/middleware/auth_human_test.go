package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/capyrpi/api/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestAuth(t *testing.T) {
	secret := "test-secret"

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
					UserID: "user-123",
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
					UserID: "user-123",
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
			handler := middleware.Auth(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// If we get here, check context
				claims, ok := middleware.GetUserClaims(r.Context())
				if ok {
					assert.Equal(t, "user-123", claims.UserID)
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
