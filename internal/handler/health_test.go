package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database/mocks"
	"github.com/capyrpi/api/internal/handler"
	"github.com/stretchr/testify/assert"
)

func TestHealth(t *testing.T) {
	// Setup
	mockQueries := mocks.NewQuerier(t)
	cfg := &config.Config{}
	h := handler.New(mockQueries, cfg)

	// Request
	req, _ := http.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	// Handler
	handler := http.HandlerFunc(h.Health)
	handler.ServeHTTP(rr, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rr.Body.String())
}
