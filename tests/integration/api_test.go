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
	"github.com/google/uuid"
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
	req, _ := http.NewRequest("GET", server.URL+"/api/v1/auth/me", nil)
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
	req, _ = http.NewRequest("POST", server.URL+"/api/v1/organizations", bytes.NewBuffer(orgBody))
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
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/organizations/%s/members", server.URL, orgID), nil)
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

func TestEventFlow(t *testing.T) {
	// Setup
	pool := testutils.SetupTestDB(t)
	defer pool.Close()

	queries := database.New(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpiryHours: 1}}
	h := handler.New(queries, cfg)
	r := router.New(h, queries, cfg.JWT.Secret, []string{})
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	ctx := context.Background()
	user, err := queries.CreateUser(ctx, database.CreateUserParams{FirstName: "Evt", LastName: "Tester", Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true}})
	require.NoError(t, err)

	// Auth
	claims := middleware.UserClaims{UserID: user.Uid.String(), RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	require.NoError(t, err)
	cookie := &http.Cookie{Name: "capy_auth", Value: tokenString}

	// Create Org
	orgBody := []byte(`{"name":"Event Org","slug":"event-org"}`)
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/organizations", bytes.NewBuffer(orgBody))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var orgResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&orgResp)
	require.NoError(t, err)
	oid := orgResp["oid"].(string)

	// Create Event
	// include org_id and some fields
	eventJSON := fmt.Sprintf(`{"org_id":"%s","location":"Auditorium","description":"Test event","event_time":"%s"}`, oid, time.Now().Add(24*time.Hour).Format(time.RFC3339))
	req, _ = http.NewRequest("POST", server.URL+"/api/v1/events", bytes.NewBuffer([]byte(eventJSON)))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var evResp dto.EventResponse
	err = json.NewDecoder(resp.Body).Decode(&evResp)
	require.NoError(t, err)

	// GET event by id
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/events/%s", server.URL, evResp.EID.String()), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// List events by org
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/events/org/%s", server.URL, oid), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var events []dto.EventResponse
	err = json.NewDecoder(resp.Body).Decode(&events)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 1)

	// Register user for event
	regBody := fmt.Sprintf(`{"uid":"%s","is_attending":true}`, user.Uid.String())
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/events/%s/register", server.URL, evResp.EID.String()), bytes.NewBuffer([]byte(regBody)))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Register without uid -> should return 400
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/events/%s/register", server.URL, evResp.EID.String()), bytes.NewBuffer([]byte(`{"is_attending":true}`)))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// List registrations
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/events/%s/registrations", server.URL, evResp.EID.String()), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var regs []dto.EventRegistrationResponse
	err = json.NewDecoder(resp.Body).Decode(&regs)
	require.NoError(t, err)
	require.Len(t, regs, 1)
	assert.Equal(t, user.Uid.String(), regs[0].UID.String())

	// Verify user events endpoint
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/users/%s/events", server.URL, user.Uid.String()), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var uevents []dto.EventResponse
	err = json.NewDecoder(resp.Body).Decode(&uevents)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(uevents), 1)

	// List all events
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/events", server.URL), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var allevents []dto.EventResponse
	err = json.NewDecoder(resp.Body).Decode(&allevents)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(allevents), 1)
}

func TestUserUpdateDelete(t *testing.T) {
	pool := testutils.SetupTestDB(t)
	defer pool.Close()

	q := database.New(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpiryHours: 1}}
	h := handler.New(q, cfg)
	r := router.New(h, q, cfg.JWT.Secret, []string{})
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	ctx := context.Background()
	user, err := q.CreateUser(ctx, database.CreateUserParams{FirstName: "U", LastName: "PD", Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true}})
	require.NoError(t, err)

	claims := middleware.UserClaims{UserID: user.Uid.String(), RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	require.NoError(t, err)
	cookie := &http.Cookie{Name: "capy_auth", Value: tokenString}

	// Update user
	updateBody := []byte(`{"first_name":"Updated","last_name":"Name"}`)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/api/v1/users/%s", server.URL, user.Uid.String()), bytes.NewBuffer(updateBody))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var ur dto.UserResponse
	err = json.NewDecoder(resp.Body).Decode(&ur)
	require.NoError(t, err)
	assert.Equal(t, "Updated", ur.FirstName)
	assert.Equal(t, "Name", ur.LastName)

	// Delete user
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/users/%s", server.URL, user.Uid.String()), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify deletion
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/users/%s", server.URL, user.Uid.String()), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	// Should be 404 or error handled
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrgMemberEndpoints(t *testing.T) {
	pool := testutils.SetupTestDB(t)
	defer pool.Close()

	q := database.New(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpiryHours: 1}}
	h := handler.New(q, cfg)
	r := router.New(h, q, cfg.JWT.Secret, []string{})
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	ctx := context.Background()
	// Create two users
	admin, err := q.CreateUser(ctx, database.CreateUserParams{FirstName: "Admin", LastName: "User", Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true}})
	require.NoError(t, err)
	member, err := q.CreateUser(ctx, database.CreateUserParams{FirstName: "Member", LastName: "User", Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true}})
	require.NoError(t, err)

	claims := middleware.UserClaims{UserID: admin.Uid.String(), RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	require.NoError(t, err)
	cookie := &http.Cookie{Name: "capy_auth", Value: tokenString}

	// Create organization (admin will be creator)
	orgBody := []byte(`{"name":"Members Org","slug":"members-org"}`)
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/organizations", bytes.NewBuffer(orgBody))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var orgResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&orgResp)
	require.NoError(t, err)
	oid := orgResp["oid"].(string)

	// Add member to org
	addBody := fmt.Sprintf(`{"uid":"%s","is_admin":false}`, member.Uid.String())
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/organizations/%s/members", server.URL, oid), bytes.NewBuffer([]byte(addBody)))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// List org members
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/organizations/%s/members", server.URL, oid), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var members []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&members)
	require.NoError(t, err)
	// Expect at least two members (creator + added)
	require.GreaterOrEqual(t, len(members), 2)

	// Verify GetUserOrganizations for member
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/users/%s/organizations", server.URL, member.Uid.String()), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var uorgs []dto.OrganizationResponse
	err = json.NewDecoder(resp.Body).Decode(&uorgs)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(uorgs), 1)
}

func TestEventUpdateDeleteFlow(t *testing.T) {
	pool := testutils.SetupTestDB(t)
	defer pool.Close()

	q := database.New(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpiryHours: 1}}
	h := handler.New(q, cfg)
	r := router.New(h, q, cfg.JWT.Secret, []string{})
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	ctx := context.Background()
	user, err := q.CreateUser(ctx, database.CreateUserParams{FirstName: "EUpd", LastName: "Tester", Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true}})
	require.NoError(t, err)

	claims := middleware.UserClaims{UserID: user.Uid.String(), RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	require.NoError(t, err)
	cookie := &http.Cookie{Name: "capy_auth", Value: tokenString}

	// Create org
	orgBody := []byte(`{"name":"EvUpd Org","slug":"evupd-org"}`)
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/organizations", bytes.NewBuffer(orgBody))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var orgResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&orgResp)
	require.NoError(t, err)
	oid := orgResp["oid"].(string)

	// Create event
	eventJSON := fmt.Sprintf(`{"org_id":"%s","location":"Room","description":"Desc","event_time":"%s"}`, oid, time.Now().Add(48*time.Hour).Format(time.RFC3339))
	req, _ = http.NewRequest("POST", server.URL+"/api/v1/events", bytes.NewBuffer([]byte(eventJSON)))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var ev dto.EventResponse
	err = json.NewDecoder(resp.Body).Decode(&ev)
	require.NoError(t, err)

	// Try creating event with missing org_id
	badEvent := []byte(`{"location":"Nowhere"}`)
	req, _ = http.NewRequest("POST", server.URL+"/api/v1/events", bytes.NewBuffer(badEvent))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Update event
	updBody := []byte(`{"location":"Updated Room","description":"Updated"}`)
	req, _ = http.NewRequest("PUT", fmt.Sprintf("%s/api/v1/events/%s", server.URL, ev.EID.String()), bytes.NewBuffer(updBody))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var evUpd dto.EventResponse
	err = json.NewDecoder(resp.Body).Decode(&evUpd)
	require.NoError(t, err)
	assert.Equal(t, "Updated Room", *evUpd.Location)

	// Unregister (will error if uid not provided) -> ensure delete works when provided
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/events/%s/register?uid=%s", server.URL, ev.EID.String(), user.Uid.String()), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	// Unregister may return 400 if not registered, but ensure handler responds (expect 404 or 204)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	// Delete event
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/events/%s", server.URL, ev.EID.String()), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestOrganizationCRUD(t *testing.T) {
	pool := testutils.SetupTestDB(t)
	defer pool.Close()

	q := database.New(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpiryHours: 1}}
	h := handler.New(q, cfg)
	r := router.New(h, q, cfg.JWT.Secret, []string{})
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	ctx := context.Background()
	creator, err := q.CreateUser(ctx, database.CreateUserParams{FirstName: "Org", LastName: "Creator", Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true}})
	require.NoError(t, err)

	claims := middleware.UserClaims{UserID: creator.Uid.String(), RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	require.NoError(t, err)
	cookie := &http.Cookie{Name: "capy_auth", Value: tokenString}

	// Create org
	orgBody := []byte(`{"name":"Crud Org","slug":"crud-org"}`)
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/organizations", bytes.NewBuffer(orgBody))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var orgResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&orgResp)
	require.NoError(t, err)
	oid := orgResp["oid"].(string)

	// Get organization
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/organizations/%s", server.URL, oid), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Update organization
	upd := []byte(`{"name":"Crud Org Updated"}`)
	req, _ = http.NewRequest("PUT", fmt.Sprintf("%s/api/v1/organizations/%s", server.URL, oid), bytes.NewBuffer(upd))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updated dto.OrganizationResponse
	err = json.NewDecoder(resp.Body).Decode(&updated)
	require.NoError(t, err)
	assert.Equal(t, "Crud Org Updated", updated.Name)

	// List organizations
	req, _ = http.NewRequest("GET", server.URL+"/api/v1/organizations", nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var orgs []dto.OrganizationResponse
	err = json.NewDecoder(resp.Body).Decode(&orgs)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(orgs), 1)

	// Add and remove member
	member, err := q.CreateUser(ctx, database.CreateUserParams{FirstName: "ToRemove", LastName: "Member", Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true}})
	require.NoError(t, err)
	addBody := fmt.Sprintf(`{"uid":"%s","is_admin":false}`, member.Uid.String())
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/organizations/%s/members", server.URL, oid), bytes.NewBuffer([]byte(addBody)))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Remove member
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/organizations/%s/members/%s", server.URL, oid, member.Uid.String()), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// List org events (should be empty)
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/organizations/%s/events", server.URL, oid), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var oevents []dto.EventResponse
	err = json.NewDecoder(resp.Body).Decode(&oevents)
	require.NoError(t, err)

	// Delete org
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/organizations/%s", server.URL, oid), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
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
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/users/%s", server.URL, createdUser.Uid.String()), nil)
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
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/users/%s", server.URL, addedUser.Uid.String()), nil)
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

func TestBotRoutes(t *testing.T) {
	pool := testutils.SetupTestDB(t)
	defer pool.Close()

	q := database.New(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpiryHours: 1}}
	h := handler.New(q, cfg)
	r := router.New(h, q, cfg.JWT.Secret, []string{})
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	ctx := context.Background()
	devUser, err := q.CreateUser(ctx, database.CreateUserParams{
		FirstName: "Bot",
		LastName:  "Admin",
		Role:      database.NullUserRole{UserRole: database.UserRoleDev, Valid: true},
	})
	require.NoError(t, err)

	member, err := q.CreateUser(ctx, database.CreateUserParams{
		FirstName: "Bot",
		LastName:  "Member",
		Role:      database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
	})
	require.NoError(t, err)

	botToken := createIntegrationBotToken(t, client, server.URL, cfg.JWT.Secret, devUser.Uid)

	meReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/bot/me", nil)
	meReq.Header.Set("X-Bot-Token", botToken)
	meResp, err := client.Do(meReq)
	require.NoError(t, err)
	defer meResp.Body.Close()
	require.Equal(t, http.StatusOK, meResp.StatusCode)

	var botMe handler.BotMeResponse
	require.NoError(t, json.NewDecoder(meResp.Body).Decode(&botMe))
	assert.Equal(t, "bot", botMe.AuthType)

	orgCreateReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/bot/organizations", bytes.NewBufferString(`{"name":"Bot Org","guild_id":123456789}`))
	orgCreateReq.Header.Set("Content-Type", "application/json")
	orgCreateReq.Header.Set("X-Bot-Token", botToken)
	orgCreateResp, err := client.Do(orgCreateReq)
	require.NoError(t, err)
	defer orgCreateResp.Body.Close()
	require.Equal(t, http.StatusCreated, orgCreateResp.StatusCode)

	var createdOrg dto.OrganizationResponse
	require.NoError(t, json.NewDecoder(orgCreateResp.Body).Decode(&createdOrg))
	assert.Equal(t, "Bot Org", createdOrg.Name)

	orgGuildReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/bot/organizations/guilds/123456789", nil)
	orgGuildReq.Header.Set("X-Bot-Token", botToken)
	orgGuildResp, err := client.Do(orgGuildReq)
	require.NoError(t, err)
	defer orgGuildResp.Body.Close()
	require.Equal(t, http.StatusOK, orgGuildResp.StatusCode)

	var guildOrg dto.BotOrganizationResponse
	require.NoError(t, json.NewDecoder(orgGuildResp.Body).Decode(&guildOrg))
	assert.Equal(t, createdOrg.OID, guildOrg.OID)
	assert.Equal(t, int64(123456789), guildOrg.GuildID)

	orgListReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/bot/organizations", nil)
	orgListReq.Header.Set("X-Bot-Token", botToken)
	orgListResp, err := client.Do(orgListReq)
	require.NoError(t, err)
	defer orgListResp.Body.Close()
	require.Equal(t, http.StatusOK, orgListResp.StatusCode)

	var orgs []dto.OrganizationResponse
	require.NoError(t, json.NewDecoder(orgListResp.Body).Decode(&orgs))
	require.NotEmpty(t, orgs)
	foundBotOrg := false
	for _, org := range orgs {
		if org.OID == createdOrg.OID {
			foundBotOrg = true
			if assert.NotNil(t, org.GuildID) {
				assert.Equal(t, int64(123456789), *org.GuildID)
			}
		}
	}
	assert.True(t, foundBotOrg)

	orgGetReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/bot/organizations/%s", server.URL, createdOrg.OID), nil)
	orgGetReq.Header.Set("X-Bot-Token", botToken)
	orgGetResp, err := client.Do(orgGetReq)
	require.NoError(t, err)
	defer orgGetResp.Body.Close()
	require.Equal(t, http.StatusOK, orgGetResp.StatusCode)

	var fetchedOrg dto.OrganizationResponse
	require.NoError(t, json.NewDecoder(orgGetResp.Body).Decode(&fetchedOrg))
	assert.Equal(t, createdOrg.OID, fetchedOrg.OID)

	missingGuildReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/bot/organizations/guilds/999999999", nil)
	missingGuildReq.Header.Set("X-Bot-Token", botToken)
	missingGuildResp, err := client.Do(missingGuildReq)
	require.NoError(t, err)
	defer missingGuildResp.Body.Close()
	require.Equal(t, http.StatusNotFound, missingGuildResp.StatusCode)

	orgUpdateReq, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/v1/bot/organizations/%s", server.URL, createdOrg.OID), bytes.NewBufferString(`{"name":"Bot Org Updated"}`))
	orgUpdateReq.Header.Set("Content-Type", "application/json")
	orgUpdateReq.Header.Set("X-Bot-Token", botToken)
	orgUpdateResp, err := client.Do(orgUpdateReq)
	require.NoError(t, err)
	defer orgUpdateResp.Body.Close()
	require.Equal(t, http.StatusOK, orgUpdateResp.StatusCode)

	var updatedOrg dto.OrganizationResponse
	require.NoError(t, json.NewDecoder(orgUpdateResp.Body).Decode(&updatedOrg))
	assert.Equal(t, "Bot Org Updated", updatedOrg.Name)

	memberListReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/bot/organizations/%s/members", server.URL, createdOrg.OID), nil)
	memberListReq.Header.Set("X-Bot-Token", botToken)
	memberListResp, err := client.Do(memberListReq)
	require.NoError(t, err)
	defer memberListResp.Body.Close()
	require.Equal(t, http.StatusOK, memberListResp.StatusCode)

	var initialMembers []dto.OrgMemberResponse
	require.NoError(t, json.NewDecoder(memberListResp.Body).Decode(&initialMembers))
	assert.Len(t, initialMembers, 0)

	addMemberReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/bot/organizations/%s/members", server.URL, createdOrg.OID), bytes.NewBufferString(fmt.Sprintf(`{"uid":"%s","is_admin":false}`, member.Uid)))
	addMemberReq.Header.Set("Content-Type", "application/json")
	addMemberReq.Header.Set("X-Bot-Token", botToken)
	addMemberResp, err := client.Do(addMemberReq)
	require.NoError(t, err)
	defer addMemberResp.Body.Close()
	require.Equal(t, http.StatusCreated, addMemberResp.StatusCode)

	memberListReq, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/bot/organizations/%s/members", server.URL, createdOrg.OID), nil)
	memberListReq.Header.Set("X-Bot-Token", botToken)
	memberListResp, err = client.Do(memberListReq)
	require.NoError(t, err)
	defer memberListResp.Body.Close()
	require.Equal(t, http.StatusOK, memberListResp.StatusCode)

	var orgMembers []dto.OrgMemberResponse
	require.NoError(t, json.NewDecoder(memberListResp.Body).Decode(&orgMembers))
	require.Len(t, orgMembers, 1)
	assert.Equal(t, member.Uid, orgMembers[0].UID)

	userReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/bot/users/%s", server.URL, member.Uid), nil)
	userReq.Header.Set("X-Bot-Token", botToken)
	userResp, err := client.Do(userReq)
	require.NoError(t, err)
	defer userResp.Body.Close()
	require.Equal(t, http.StatusOK, userResp.StatusCode)

	var user dto.UserResponse
	require.NoError(t, json.NewDecoder(userResp.Body).Decode(&user))
	assert.Equal(t, member.Uid, user.UID)

	userOrgsReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/bot/users/%s/organizations", server.URL, member.Uid), nil)
	userOrgsReq.Header.Set("X-Bot-Token", botToken)
	userOrgsResp, err := client.Do(userOrgsReq)
	require.NoError(t, err)
	defer userOrgsResp.Body.Close()
	require.Equal(t, http.StatusOK, userOrgsResp.StatusCode)

	var userOrgs []dto.OrganizationResponse
	require.NoError(t, json.NewDecoder(userOrgsResp.Body).Decode(&userOrgs))
	require.Len(t, userOrgs, 1)
	assert.Equal(t, createdOrg.OID, userOrgs[0].OID)

	eventTime := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	eventCreateReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/bot/events", bytes.NewBufferString(fmt.Sprintf(`{"org_id":"%s","location":"Bot Hall","description":"Bot Event","event_time":"%s"}`, createdOrg.OID, eventTime)))
	eventCreateReq.Header.Set("Content-Type", "application/json")
	eventCreateReq.Header.Set("X-Bot-Token", botToken)
	eventCreateResp, err := client.Do(eventCreateReq)
	require.NoError(t, err)
	defer eventCreateResp.Body.Close()
	require.Equal(t, http.StatusCreated, eventCreateResp.StatusCode)

	var createdEvent dto.EventResponse
	require.NoError(t, json.NewDecoder(eventCreateResp.Body).Decode(&createdEvent))
	require.NotEqual(t, uuid.Nil, createdEvent.EID)

	eventListReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/bot/events", nil)
	eventListReq.Header.Set("X-Bot-Token", botToken)
	eventListResp, err := client.Do(eventListReq)
	require.NoError(t, err)
	defer eventListResp.Body.Close()
	require.Equal(t, http.StatusOK, eventListResp.StatusCode)

	var events []dto.EventResponse
	require.NoError(t, json.NewDecoder(eventListResp.Body).Decode(&events))
	require.NotEmpty(t, events)

	eventGetReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/bot/events/%s", server.URL, createdEvent.EID), nil)
	eventGetReq.Header.Set("X-Bot-Token", botToken)
	eventGetResp, err := client.Do(eventGetReq)
	require.NoError(t, err)
	defer eventGetResp.Body.Close()
	require.Equal(t, http.StatusOK, eventGetResp.StatusCode)

	var fetchedEvent dto.EventResponse
	require.NoError(t, json.NewDecoder(eventGetResp.Body).Decode(&fetchedEvent))
	assert.Equal(t, createdEvent.EID, fetchedEvent.EID)

	eventUpdateReq, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/v1/bot/events/%s", server.URL, createdEvent.EID), bytes.NewBufferString(`{"location":"Bot Hall Updated","description":"Updated Bot Event"}`))
	eventUpdateReq.Header.Set("Content-Type", "application/json")
	eventUpdateReq.Header.Set("X-Bot-Token", botToken)
	eventUpdateResp, err := client.Do(eventUpdateReq)
	require.NoError(t, err)
	defer eventUpdateResp.Body.Close()
	require.Equal(t, http.StatusOK, eventUpdateResp.StatusCode)

	var updatedEvent dto.EventResponse
	require.NoError(t, json.NewDecoder(eventUpdateResp.Body).Decode(&updatedEvent))
	require.NotNil(t, updatedEvent.Location)
	assert.Equal(t, "Bot Hall Updated", *updatedEvent.Location)

	registerReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/bot/events/%s/register", server.URL, createdEvent.EID), bytes.NewBufferString(fmt.Sprintf(`{"uid":"%s","is_attending":true}`, member.Uid)))
	registerReq.Header.Set("Content-Type", "application/json")
	registerReq.Header.Set("X-Bot-Token", botToken)
	registerResp, err := client.Do(registerReq)
	require.NoError(t, err)
	defer registerResp.Body.Close()
	require.Equal(t, http.StatusCreated, registerResp.StatusCode)

	regListReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/bot/events/%s/registrations", server.URL, createdEvent.EID), nil)
	regListReq.Header.Set("X-Bot-Token", botToken)
	regListResp, err := client.Do(regListReq)
	require.NoError(t, err)
	defer regListResp.Body.Close()
	require.Equal(t, http.StatusOK, regListResp.StatusCode)

	var regs []dto.EventRegistrationResponse
	require.NoError(t, json.NewDecoder(regListResp.Body).Decode(&regs))
	require.Len(t, regs, 1)
	assert.Equal(t, member.Uid, regs[0].UID)

	userEventsReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/bot/users/%s/events", server.URL, member.Uid), nil)
	userEventsReq.Header.Set("X-Bot-Token", botToken)
	userEventsResp, err := client.Do(userEventsReq)
	require.NoError(t, err)
	defer userEventsResp.Body.Close()
	require.Equal(t, http.StatusOK, userEventsResp.StatusCode)

	var userEvents []dto.EventResponse
	require.NoError(t, json.NewDecoder(userEventsResp.Body).Decode(&userEvents))
	require.Len(t, userEvents, 1)
	assert.Equal(t, createdEvent.EID, userEvents[0].EID)

	unregisterReq, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/bot/events/%s/register?uid=%s", server.URL, createdEvent.EID, member.Uid), nil)
	unregisterReq.Header.Set("X-Bot-Token", botToken)
	unregisterResp, err := client.Do(unregisterReq)
	require.NoError(t, err)
	defer unregisterResp.Body.Close()
	require.Equal(t, http.StatusNoContent, unregisterResp.StatusCode)

	removeMemberReq, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/bot/organizations/%s/members/%s", server.URL, createdOrg.OID, member.Uid), nil)
	removeMemberReq.Header.Set("X-Bot-Token", botToken)
	removeMemberResp, err := client.Do(removeMemberReq)
	require.NoError(t, err)
	defer removeMemberResp.Body.Close()
	require.Equal(t, http.StatusNoContent, removeMemberResp.StatusCode)

	deleteEventReq, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/bot/events/%s", server.URL, createdEvent.EID), nil)
	deleteEventReq.Header.Set("X-Bot-Token", botToken)
	deleteEventResp, err := client.Do(deleteEventReq)
	require.NoError(t, err)
	defer deleteEventResp.Body.Close()
	require.Equal(t, http.StatusNoContent, deleteEventResp.StatusCode)

	deleteOrgReq, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/bot/organizations/%s", server.URL, createdOrg.OID), nil)
	deleteOrgReq.Header.Set("X-Bot-Token", botToken)
	deleteOrgResp, err := client.Do(deleteOrgReq)
	require.NoError(t, err)
	defer deleteOrgResp.Body.Close()
	require.Equal(t, http.StatusNoContent, deleteOrgResp.StatusCode)
}

func createIntegrationBotToken(t *testing.T, client *http.Client, serverURL, jwtSecret string, devUserID uuid.UUID) string {
	t.Helper()

	claims := middleware.UserClaims{
		UserID: devUserID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(jwtSecret))
	require.NoError(t, err)

	req, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/bot/tokens", bytes.NewBufferString(`{"name":"integration-bot"}`))
	req.Header.Set("Authorization", "Bearer "+tokenString)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var botTokenResp handler.BotTokenResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&botTokenResp))
	require.NotEmpty(t, botTokenResp.Token)

	return botTokenResp.Token
}
