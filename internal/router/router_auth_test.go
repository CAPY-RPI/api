package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/database/mocks"
	"github.com/capyrpi/api/internal/handler"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/capyrpi/api/internal/router"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestBotTokenLifecycle(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	routerUnderTest := newTestRouter(mockQueries)

	devID := uuid.New()
	tokenID := uuid.New()
	var storedHash string
	devUser := database.User{
		Uid:  devID,
		Role: database.NullUserRole{UserRole: database.UserRoleDev, Valid: true},
	}

	mockQueries.On("GetUserByID", mock.Anything, devID).Return(devUser, nil).Times(3)

	mockQueries.On("CreateBotToken", mock.Anything, mock.MatchedBy(func(arg database.CreateBotTokenParams) bool {
		storedHash = arg.TokenHash
		return arg.Name == "deploy-bot" && arg.CreatedBy == devID && arg.ExpiresAt.Valid
	})).Return(database.BotToken{
		TokenID:   tokenID,
		Name:      "deploy-bot",
		CreatedAt: pgTimestamp(time.Now().UTC()),
		ExpiresAt: pgTimestamp(time.Now().UTC().Add(24 * time.Hour)),
		IsActive:  pgBool(true),
	}, nil).Once()

	createReqBody := []byte(`{"name":"deploy-bot","expires_at":"2030-01-02T03:04:05Z"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/bot/tokens", bytes.NewReader(createReqBody))
	createReq.Header.Set("Authorization", "Bearer "+makeJWT(t, devID, string(database.UserRoleDev)))
	createRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(createRes, createReq)
	require.Equal(t, http.StatusCreated, createRes.Code)

	var createdTokenResp handler.BotTokenResponse
	require.NoError(t, json.Unmarshal(createRes.Body.Bytes(), &createdTokenResp))
	assert.Equal(t, tokenID, createdTokenResp.TokenID)
	assert.NotEmpty(t, createdTokenResp.Token)

	mockQueries.On("ListBotTokens", mock.Anything).Return([]database.ListBotTokensRow{
		{
			TokenID:   tokenID,
			Name:      "deploy-bot",
			CreatedAt: pgTimestamp(time.Now().UTC()),
			ExpiresAt: pgTimestamp(time.Now().UTC().Add(24 * time.Hour)),
			IsActive:  pgBool(true),
		},
	}, nil).Once()

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/bot/tokens", nil)
	listReq.Header.Set("Authorization", "Bearer "+makeJWT(t, devID, string(database.UserRoleDev)))
	listRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(listRes, listReq)
	require.Equal(t, http.StatusOK, listRes.Code)

	var listedTokens []map[string]any
	require.NoError(t, json.Unmarshal(listRes.Body.Bytes(), &listedTokens))
	require.Len(t, listedTokens, 1)
	_, tokenPresent := listedTokens[0]["token"]
	assert.False(t, tokenPresent)

	mockQueries.On("GetBotTokenByID", mock.Anything, tokenID).Return(func(context.Context, uuid.UUID) database.BotToken {
		return database.BotToken{
			TokenID:   tokenID,
			TokenHash: storedHash,
			Name:      "deploy-bot",
			ExpiresAt: pgTimestamp(time.Now().UTC().Add(24 * time.Hour)),
			IsActive:  pgBool(true),
		}
	}, nil).Once()
	mockQueries.On("UpdateBotTokenLastUsed", mock.Anything, tokenID).Return(nil).Once()

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/bot/me", nil)
	meReq.Header.Set("X-Bot-Token", createdTokenResp.Token)
	meRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(meRes, meReq)
	require.Equal(t, http.StatusOK, meRes.Code)

	var meResp handler.BotMeResponse
	require.NoError(t, json.Unmarshal(meRes.Body.Bytes(), &meResp))
	assert.Equal(t, tokenID, meResp.TokenID)
	assert.Equal(t, "deploy-bot", meResp.Name)
	assert.Equal(t, "bot", meResp.AuthType)

	mockQueries.On("RevokeBotToken", mock.Anything, tokenID).Return(nil).Once()

	revokeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/bot/tokens/"+tokenID.String(), nil)
	revokeReq.Header.Set("Authorization", "Bearer "+makeJWT(t, devID, string(database.UserRoleDev)))
	revokeRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(revokeRes, revokeReq)
	require.Equal(t, http.StatusNoContent, revokeRes.Code)

	mockQueries.On("GetBotTokenByID", mock.Anything, tokenID).Return(database.BotToken{
		TokenID:   tokenID,
		TokenHash: storedHash,
		Name:      "deploy-bot",
		IsActive:  pgBool(false),
	}, nil).Once()

	revokedReq := httptest.NewRequest(http.MethodGet, "/api/v1/bot/me", nil)
	revokedReq.Header.Set("X-Bot-Token", createdTokenResp.Token)
	revokedRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(revokedRes, revokedReq)
	assert.Equal(t, http.StatusUnauthorized, revokedRes.Code)
}

func TestBotRouteAuthBoundaries(t *testing.T) {
	t.Run("MissingBotHeaderFails", func(t *testing.T) {
		mockQueries := mocks.NewQuerier(t)
		routerUnderTest := newTestRouter(mockQueries)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/bot/me", nil)
		res := httptest.NewRecorder()
		routerUnderTest.ServeHTTP(res, req)

		assert.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("InvalidBotTokenFails", func(t *testing.T) {
		mockQueries := mocks.NewQuerier(t)
		routerUnderTest := newTestRouter(mockQueries)

		tokenID := uuid.New()
		hash, err := bcryptHash("correct-secret")
		require.NoError(t, err)

		mockQueries.On("GetBotTokenByID", mock.Anything, tokenID).Return(database.BotToken{
			TokenID:   tokenID,
			TokenHash: hash,
			Name:      "deploy-bot",
			IsActive:  pgBool(true),
		}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/bot/me", nil)
		req.Header.Set("X-Bot-Token", tokenID.String()+".wrong-secret")
		res := httptest.NewRecorder()
		routerUnderTest.ServeHTTP(res, req)

		assert.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("ExpiredBotTokenFails", func(t *testing.T) {
		mockQueries := mocks.NewQuerier(t)
		routerUnderTest := newTestRouter(mockQueries)

		tokenID := uuid.New()
		hash, err := bcryptHash("secret")
		require.NoError(t, err)

		mockQueries.On("GetBotTokenByID", mock.Anything, tokenID).Return(database.BotToken{
			TokenID:   tokenID,
			TokenHash: hash,
			Name:      "deploy-bot",
			ExpiresAt: pgTimestamp(time.Now().UTC().Add(-time.Minute)),
			IsActive:  pgBool(true),
		}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/bot/me", nil)
		req.Header.Set("X-Bot-Token", tokenID.String()+".secret")
		res := httptest.NewRecorder()
		routerUnderTest.ServeHTTP(res, req)

		assert.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("HumanEndpointsRejectBotToken", func(t *testing.T) {
		mockQueries := mocks.NewQuerier(t)
		routerUnderTest := newTestRouter(mockQueries)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("X-Bot-Token", uuid.New().String()+".secret")
		res := httptest.NewRecorder()
		routerUnderTest.ServeHTTP(res, req)

		assert.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("BotEndpointsRejectHumanJWT", func(t *testing.T) {
		mockQueries := mocks.NewQuerier(t)
		routerUnderTest := newTestRouter(mockQueries)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/bot/me", nil)
		req.Header.Set("Authorization", "Bearer "+makeJWT(t, uuid.New(), string(database.UserRoleDev)))
		res := httptest.NewRecorder()
		routerUnderTest.ServeHTTP(res, req)

		assert.Equal(t, http.StatusUnauthorized, res.Code)
	})
}

func TestBotTokenManagementRequiresDev(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	routerUnderTest := newTestRouter(mockQueries)

	userID := uuid.New()
	tokenID := uuid.New()
	studentUser := database.User{
		Uid:  userID,
		Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
	}

	mockQueries.On("GetUserByID", mock.Anything, userID).Return(studentUser, nil).Times(3)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/bot/tokens", bytes.NewBufferString(`{"name":"deploy-bot"}`))
	createReq.Header.Set("Authorization", "Bearer "+makeJWT(t, userID, string(database.UserRoleStudent)))
	createRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(createRes, createReq)
	assert.Equal(t, http.StatusForbidden, createRes.Code)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/bot/tokens", nil)
	listReq.Header.Set("Authorization", "Bearer "+makeJWT(t, userID, string(database.UserRoleStudent)))
	listRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(listRes, listReq)
	assert.Equal(t, http.StatusForbidden, listRes.Code)

	revokeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/bot/tokens/"+tokenID.String(), nil)
	revokeReq.Header.Set("Authorization", "Bearer "+makeJWT(t, userID, string(database.UserRoleStudent)))
	revokeRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(revokeRes, revokeReq)
	assert.Equal(t, http.StatusForbidden, revokeRes.Code)
}

func TestBotTokenManagementUsesDatabaseRole(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	routerUnderTest := newTestRouter(mockQueries)

	devID := uuid.New()
	devUser := database.User{
		Uid:  devID,
		Role: database.NullUserRole{UserRole: database.UserRoleDev, Valid: true},
	}

	mockQueries.On("GetUserByID", mock.Anything, devID).Return(devUser, nil).Once()
	mockQueries.On("ListBotTokens", mock.Anything).Return([]database.ListBotTokensRow{}, nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bot/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+makeJWT(t, devID, string(database.UserRoleStudent)))
	res := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
}

func TestPublicCollectionRoutesDoNotRequireAuth(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	routerUnderTest := newTestRouter(mockQueries)

	mockQueries.On("ListOrganizations", mock.Anything, mock.MatchedBy(func(arg database.ListOrganizationsParams) bool {
		return arg.Limit == 20 && arg.Offset == 0
	})).Return([]database.ListOrganizationsRow{}, nil).Once()

	mockQueries.On("ListEvents", mock.Anything, mock.MatchedBy(func(arg database.ListEventsParams) bool {
		return arg.Limit == 20 && arg.Offset == 0
	})).Return([]database.EventsWithOrgID{}, nil).Once()

	orgReq := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
	orgRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(orgRes, orgReq)
	assert.Equal(t, http.StatusOK, orgRes.Code)

	eventReq := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	eventRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(eventRes, eventReq)
	assert.Equal(t, http.StatusOK, eventRes.Code)

	protectedReq := httptest.NewRequest(http.MethodPost, "/api/v1/organizations", bytes.NewBufferString(`{"name":"Still Protected"}`))
	protectedRes := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(protectedRes, protectedReq)
	assert.Equal(t, http.StatusUnauthorized, protectedRes.Code)
}

func TestBotGuildLookupRequiresBotAuth(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	routerUnderTest := newTestRouter(mockQueries)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bot/organizations/guilds/123456789", nil)
	res := httptest.NewRecorder()
	routerUnderTest.ServeHTTP(res, req)

	assert.Equal(t, http.StatusUnauthorized, res.Code)
}

func newTestRouter(queries database.Querier) http.Handler {
	cfg := &config.Config{
		Env: "test",
		JWT: config.JWTConfig{
			Secret:      "test-secret",
			ExpiryHours: 24,
		},
	}

	h := handler.New(queries, cfg)
	return router.New(h, queries, cfg.JWT.Secret, nil)
}

func makeJWT(t *testing.T, userID uuid.UUID, role string) string {
	t.Helper()

	claims := middleware.UserClaims{
		UserID: userID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return tokenString
}

func pgBool(v bool) pgtype.Bool {
	return pgtype.Bool{Bool: v, Valid: true}
}

func pgTimestamp(v time.Time) pgtype.Timestamp {
	return pgtype.Timestamp{Time: v, Valid: true}
}

func bcryptHash(secret string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
