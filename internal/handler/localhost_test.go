package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/capyrpi/api/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestLocalhostDetection(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		host     string
		expected bool
	}{
		{"localhost in dev", "development", "localhost:8080", true},
		{"127.0.0.1 in dev", "development", "127.0.0.1:8080", true},
		{"production host in dev", "development", "api.capyrpi.org", false},
		{"localhost in prod", "production", "localhost:8080", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{Config: &config.Config{Env: tt.env}}
			r := httptest.NewRequest("GET", "/", nil)
			r.Host = tt.host
			assert.Equal(t, tt.expected, h.isLocalhost(r))
		})
	}
}

func TestGetCookieDomain(t *testing.T) {
	h := &Handler{Config: &config.Config{
		Env: "development",
		Cookie: config.CookieConfig{Domain: "capyrpi.org"},
	}}

	rLocal := httptest.NewRequest("GET", "/", nil)
	rLocal.Host = "localhost:8080"
	assert.Equal(t, "localhost", h.getCookieDomain(rLocal))

	rProd := httptest.NewRequest("GET", "/", nil)
	rProd.Host = "api.capyrpi.org"
	assert.Equal(t, "capyrpi.org", h.getCookieDomain(rProd))
}

func TestGetOAuthRedirectURL(t *testing.T) {
	h := &Handler{Config: &config.Config{
		Env: "development",
	}}

	providerURL := "https://api.capyrpi.org/api/v1/auth/google/callback"

	rLocal := httptest.NewRequest("GET", "/", nil)
	rLocal.Host = "localhost:8080"
	assert.Equal(t, "http://localhost:8080/api/v1/auth/google/callback", h.getOAuthRedirectURL(rLocal, providerURL))

	rProd := httptest.NewRequest("GET", "/", nil)
	rProd.Host = "api.capyrpi.org"
	assert.Equal(t, "", h.getOAuthRedirectURL(rProd, providerURL))
}
