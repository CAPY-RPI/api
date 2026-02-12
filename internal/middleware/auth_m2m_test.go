package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/capyrpi/api/internal/database/mocks"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestM2MAuth_MissingHeaders(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	handler := middleware.M2MAuth(mockQueries)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/bot/me", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "Missing API key header")
}

func TestM2MAuth_AcceptsXAPIKeyHeader(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	mockQueries.On("ListBotTokens", mock.Anything).Return(nil, errors.New("db down")).Once()

	handler := middleware.M2MAuth(mockQueries)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/bot/me", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "Internal server error")
}

func TestM2MAuth_AcceptsXBotTokenHeader(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	mockQueries.On("ListBotTokens", mock.Anything).Return(nil, errors.New("db down")).Once()

	handler := middleware.M2MAuth(mockQueries)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/bot/me", nil)
	req.Header.Set("X-Bot-Token", "test-bot-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "Internal server error")
}
