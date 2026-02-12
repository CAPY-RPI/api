//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/handler"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/capyrpi/api/internal/router"
	"github.com/capyrpi/api/internal/testutils"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullAPI(t *testing.T) {
	// 1. Infrastructure Setup
	pool := testutils.SetupTestDB(t)
	defer pool.Close()

	queries := database.New(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "test-secret", ExpiryHours: 1},
	}

	h := handler.New(queries, cfg)
	r := router.New(h, queries, cfg.JWT.Secret, []string{})
	server := httptest.NewServer(r)
	defer server.Close()

	client := server.Client()

	// 2. Create User (Directly in DB to simulate OAuth login flow having happened)
	// We need a user to authenticate as
	ctx := context.Background()
	user, err := queries.CreateUser(ctx, database.CreateUserParams{
		FirstName: "Integration",
		LastName:  "Tester",
		Role:      database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
	})
	require.NoError(t, err)

	// 3. Generate Auth Token
	claims := middleware.UserClaims{
		UserID: user.Uid.String(),
		Role:   string(user.Role.UserRole),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	require.NoError(t, err)

	// Cookie
	cookie := &http.Cookie{
		Name:  "capy_auth",
		Value: tokenString,
	}

	// 4. Test: Get Me (Protected Route)
	req, _ := http.NewRequest("GET", server.URL+"/v1/auth/me", nil)
	req.AddCookie(cookie)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var meResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&meResponse)
	require.NoError(t, err)
	assert.Equal(t, user.Uid.String(), meResponse["uid"])

	// 5. Test: Create Organization
	orgBody := []byte(`{"name": "Test Org", "slug": "test-org"}`)
	req, _ = http.NewRequest("POST", server.URL+"/v1/organizations", bytes.NewBuffer(orgBody))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var orgResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&orgResponse)
	require.NoError(t, err)
	assert.Equal(t, "Test Org", orgResponse["name"])

	// Verify User is Member of Org
	orgID := orgResponse["oid"].(string)
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/v1/organizations/%s/members", server.URL, orgID), nil)
	req.AddCookie(cookie)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var members []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&members)
	require.NoError(t, err)
	assert.Len(t, members, 1)
	assert.Equal(t, user.Uid.String(), members[0]["uid"])
}
