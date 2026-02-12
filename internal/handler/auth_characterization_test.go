package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/database/mocks"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestRespondWithCloseWindow_Contract verifies the exact HTML response contract
// used by the frontend popup.
func TestRespondWithCloseWindow_Contract(t *testing.T) {
	h := &Handler{}
	rr := httptest.NewRecorder()
	h.respondWithCloseWindow(rr)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))

	body := rr.Body.String()
	assert.Contains(t, body, "<!DOCTYPE html>")
	assert.Contains(t, body, "window.close()")
	assert.Contains(t, body, "Login Successful")
}

// TestSetAuthCookie_Contract verifies cookie attributes are preserved.
func TestSetAuthCookie_Contract(t *testing.T) {
	cfg := &config.Config{
		Cookie: config.CookieConfig{
			Domain: "example.com",
			Secure: true,
		},
		JWT: config.JWTConfig{
			ExpiryHours: 24,
		},
	}
	h := &Handler{Config: cfg}
	rr := httptest.NewRecorder()
	h.setAuthCookie(rr, "test-token")

	cookies := rr.Result().Cookies()
	requireCookie(t, cookies, "capy_auth", func(c *http.Cookie) {
		assert.Equal(t, "test-token", c.Value)
		assert.Equal(t, "/", c.Path)
		assert.Equal(t, "example.com", c.Domain)
		assert.True(t, c.HttpOnly)
		assert.True(t, c.Secure)
		assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
		assert.Equal(t, 24*3600, c.MaxAge)
	})
}

// TestLogout_Contract verifies logout behavior (cookie clearing).
func TestLogout_Contract(t *testing.T) {
	cfg := &config.Config{
		Cookie: config.CookieConfig{
			Domain: "example.com",
			Secure: true,
		},
	}
	h := &Handler{Config: cfg}

	req := httptest.NewRequest("POST", "/auth/logout", nil)
	rr := httptest.NewRecorder()

	h.Logout(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	cookies := rr.Result().Cookies()
	requireCookie(t, cookies, "capy_auth", func(c *http.Cookie) {
		assert.Equal(t, "", c.Value)
		assert.Equal(t, -1, c.MaxAge) // Cleared
		assert.Equal(t, "/", c.Path)
		assert.Equal(t, "example.com", c.Domain)
	})
}

// TestSetStateCookie_Contract verifies oauth state cookie attributes.
func TestSetStateCookie_Contract(t *testing.T) {
	cfg := &config.Config{
		Cookie: config.CookieConfig{
			Domain: "example.com",
			Secure: true,
		},
	}
	h := &Handler{Config: cfg}
	rr := httptest.NewRecorder()
	h.setStateCookie(rr, "random-state")

	cookies := rr.Result().Cookies()
	requireCookie(t, cookies, "oauth_state", func(c *http.Cookie) {
		assert.Equal(t, "random-state", c.Value)
		assert.Equal(t, "/v1/auth", c.Path) // Specific path for state
		assert.Equal(t, "example.com", c.Domain)
		assert.True(t, c.HttpOnly)
		assert.True(t, c.Secure)
		assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
		assert.Equal(t, 300, c.MaxAge) // 5 minutes
	})
}

// TestVerifyStateCookie_Contract verifies validation and clearing.
func TestVerifyStateCookie_Contract(t *testing.T) {
	cfg := &config.Config{
		Cookie: config.CookieConfig{
			Domain: "example.com",
			Secure: true,
		},
	}
	h := &Handler{Config: cfg}

	// Case 1: Valid match
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "valid-state"})
	rr := httptest.NewRecorder()

	valid := h.verifyStateCookie(rr, req, "valid-state")
	assert.True(t, valid)

	// Should clear cookie
	cookies := rr.Result().Cookies()
	requireCookie(t, cookies, "oauth_state", func(c *http.Cookie) {
		assert.Equal(t, "", c.Value)
		assert.Equal(t, -1, c.MaxAge)
	})

	// Case 2: Mismatch
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "valid-state"})
	rr = httptest.NewRecorder()

	valid = h.verifyStateCookie(rr, req, "invalid-state")
	assert.False(t, valid)
	// Even on mismatch, it should arguably clear or at least fail safely.
	// Current impl clears it unconditionally.
	cookies = rr.Result().Cookies()
	requireCookie(t, cookies, "oauth_state", func(c *http.Cookie) {
		assert.Equal(t, -1, c.MaxAge)
	})
}

// TestRefreshToken_Contract verifies refresh flow logic including cookie set.
func TestRefreshToken_Contract(t *testing.T) {
	uid := uuid.New()
	mockQueries := mocks.NewQuerier(t)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-secret",
			ExpiryHours: 1,
		},
		Cookie: config.CookieConfig{
			Domain: "localhost",
		},
	}
	h := New(mockQueries, cfg)

	// Mock DB lookup
	mockQueries.On("GetUserByID", mock.Anything, uid).Return(database.User{
		Uid:       uid,
		FirstName: "Refresh",
		LastName:  "User",
		Role:      database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
	}, nil)

	req := httptest.NewRequest("POST", "/auth/refresh", nil)
	// Inject claims via context (simulating middleware)
	claims := &middleware.UserClaims{
		UserID: uid.String(),
		Role:   "student",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.RefreshToken(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify new cookie set
	cookies := rr.Result().Cookies()
	requireCookie(t, cookies, "capy_auth", func(c *http.Cookie) {
		assert.NotEmpty(t, c.Value)
		assert.Equal(t, 3600, c.MaxAge)
	})
}

func requireCookie(t *testing.T, cookies []*http.Cookie, name string, check func(*http.Cookie)) {
	t.Helper()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == name {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatalf("Cookie %s not found", name)
	}
	check(found)
}
