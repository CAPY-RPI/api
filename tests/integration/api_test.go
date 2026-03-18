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
