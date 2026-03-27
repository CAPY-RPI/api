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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkFlow(t *testing.T) {
	// 1. Setup DB and Server
	pool := testutils.SetupTestDB(t)
	defer pool.Close()

	queries := database.New(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpiryHours: 1}}
	h := handler.New(queries, cfg)
	r := router.New(h, queries, cfg.JWT.Secret, []string{})
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	// Disable redirects on the client so we can test the 302 response directly
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	ctx := context.Background()

	// 2. Create User
	user, err := queries.CreateUser(ctx, database.CreateUserParams{
		FirstName: "Link",
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
	cookie := &http.Cookie{Name: "capy_auth", Value: tokenString}

	// 4. Create Organization
	orgBody := []byte(`{"name":"Link Org","slug":"link-org"}`)
	req, _ := http.NewRequest("POST", server.URL+"/v1/organizations", bytes.NewBuffer(orgBody))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var orgResp dto.OrganizationResponse
	err = json.NewDecoder(resp.Body).Decode(&orgResp)
	require.NoError(t, err)
	oid := orgResp.OID.String()

	// 5. Create Link
	linkReq := dto.CreateLinkRequest{
		EndpointURL: "my-promo",
		DestURL:     "https://example.com/dest",
		OrgID:       orgResp.OID,
	}
	linkBody, _ := json.Marshal(linkReq)
	req, _ = http.NewRequest("POST", server.URL+"/v1/links", bytes.NewBuffer(linkBody))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var linkResp dto.LinkResponse
	err = json.NewDecoder(resp.Body).Decode(&linkResp)
	require.NoError(t, err)
	require.Equal(t, "my-promo", linkResp.EndpointURL)
	lid := linkResp.LID.String()

	// 6. Resolve Link (Simulate user clicking it)
	req, _ = http.NewRequest("GET", server.URL+"/r/my-promo", nil)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check redirect status and location
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "https://example.com/dest", resp.Header.Get("Location"))

	// Give the async goroutine a tiny bit of time to log the visit in DB
	time.Sleep(100 * time.Millisecond)

	// 7. Check Visit Count
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/v1/links/%s/visits", server.URL, lid), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var visitResp dto.VisitCountResponse
	err = json.NewDecoder(resp.Body).Decode(&visitResp)
	require.NoError(t, err)
	assert.Equal(t, int64(1), visitResp.Count)

	// 8. Update Link
	updateReq := dto.UpdateLinkRequest{
		EndpointURL: nil,
		DestURL:     toPtr("https://example.com/updated"),
	}
	updateBody, _ := json.Marshal(updateReq)
	req, _ = http.NewRequest("PUT", fmt.Sprintf("%s/v1/links/%s", server.URL, lid), bytes.NewBuffer(updateBody))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var updatedLink dto.LinkResponse
	err = json.NewDecoder(resp.Body).Decode(&updatedLink)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/updated", updatedLink.DestURL)

	// 9. List Org Links
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/v1/organizations/%s/links", server.URL, oid), nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var links []dto.LinkResponse
	err = json.NewDecoder(resp.Body).Decode(&links)
	require.NoError(t, err)
	require.Len(t, links, 1)

	// 10. Get QR Code
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/v1/links/%s/qrcode", server.URL, lid), nil)
	// We don't usually need auth for QR code, but the endpoint currently requires CookieAuth based on swagger tags
	// Actually looking at router.go, /links/{lid}/qrcode is under protected group.
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
}

func toPtr(s string) *string {
	return &s
}
