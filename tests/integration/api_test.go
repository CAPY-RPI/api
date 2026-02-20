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
	"github.com/capyrpi/api/internal/dto"
	"github.com/capyrpi/api/internal/handler"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/capyrpi/api/internal/router"
	"github.com/capyrpi/api/internal/testutils"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

var testUserParams = database.CreateUserParams{
	FirstName:     "Test",
	LastName:      "User",
	PersonalEmail: pgtype.Text{String: "testuser@gmail.com", Valid: true},
	SchoolEmail:   pgtype.Text{String: "testuser@rpi.edu", Valid: true},
	Phone:         pgtype.Text{String: "555-555-5555", Valid: true},
	GradYear:      pgtype.Int4{Int32: 2027, Valid: true},
	Role:          database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
}

func TestAddUser(t *testing.T) {
	// Spin up container
	pool := testutils.SetupTestDB(t)
	defer pool.Close()
	q := database.New(pool)
	ctx := context.Background()

	// Create user in DB so we can authenticate as them
	createdUser, err := q.CreateUser(ctx, testUserParams)
	require.NoError(t, err)

	// Start API server
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpiryHours: 1}}
	h := handler.New(q, cfg)
	r := router.New(h, q, cfg.JWT.Secret, []string{})
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	// Generate auth token for created user
	claims := middleware.UserClaims{
		UserID: createdUser.Uid.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	require.NoError(t, err)

	cookie := &http.Cookie{Name: "capy_auth", Value: tokenString}

	// Fetch user via API
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/v1/users/%s", server.URL, createdUser.Uid.String()), nil)
	req.AddCookie(cookie)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var ur dto.UserResponse
	err = json.NewDecoder(resp.Body).Decode(&ur)
	require.NoError(t, err)
	assert.Equal(t, "Test", ur.FirstName)
	assert.Equal(t, "User", ur.LastName)
	if ur.Phone != nil {
		assert.Equal(t, "555-555-5555", *ur.Phone)
	}
	if ur.GradYear != nil {
		assert.Equal(t, 2027, *ur.GradYear)
	}
	assert.Equal(t, string(database.UserRoleStudent), ur.Role)
}

func TestAddDuplicateUser(t *testing.T) {
	// Spin up container
	pool := testutils.SetupTestDB(t)
	defer pool.Close()
	q := database.New(pool)
	ctx := context.Background()

	addedUser, err := q.CreateUser(ctx, testUserParams)
	require.NoError(t, err)

	// Start API server
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpiryHours: 1}}
	h := handler.New(q, cfg)
	r := router.New(h, q, cfg.JWT.Secret, []string{})
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	// Auth as the added user
	claims := middleware.UserClaims{
		UserID: addedUser.Uid.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	require.NoError(t, err)
	cookie := &http.Cookie{Name: "capy_auth", Value: tokenString}

	// Fetch via API and verify UID matches
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/v1/users/%s", server.URL, addedUser.Uid.String()), nil)
	req.AddCookie(cookie)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var ur dto.UserResponse
	err = json.NewDecoder(resp.Body).Decode(&ur)
	require.NoError(t, err)
	// UID equality check
	assert.Equal(t, addedUser.Uid.String(), ur.UID.String())

	// Attempt duplicate creation at DB level
	_, err = q.CreateUser(ctx, testUserParams)
	require.Error(t, err)

	schoolUser, err := q.GetUserByEmail(ctx, pgtype.Text{String: "testuser@rpi.edu", Valid: true})
	assert.Equal(t, addedUser.Uid, schoolUser.Uid)
}
