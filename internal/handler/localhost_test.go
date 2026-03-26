package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/capyrpi/api/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestDevDetection(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		host     string
		expected bool
	}{
		{"dev environment", "development", "api.capyrpi.org", true},
		{"staging environment", "staging", "api.capyrpi.org", true},
		{"empty environment", "", "api.capyrpi.org", true},
		{"production env with dev host", "production", "dev.capyrpi.org", true},
		{"production env with localhost", "production", "localhost:8080", true},
		{"production env with dev forwarded host", "production", "api.capyrpi.org", true},
		{"production env with X-Dev-Host", "production", "api.capyrpi.org", true},
		{"production environment", "production", "api.capyrpi.org", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{Config: &config.Config{Env: tt.env}}
			r := httptest.NewRequest("GET", "/", nil)
			r.Host = tt.host
			if tt.name == "production env with dev forwarded host" {
				r.Header.Set("X-Forwarded-Host", "dev.localhost")
			}
			if tt.name == "production env with X-Dev-Host" {
				r.Header.Set("X-Dev-Host", "localhost:5173")
			}
			assert.Equal(t, tt.expected, h.isDev(r))
		})
	}
}

func TestGetCookieDomain(t *testing.T) {
	h := &Handler{Config: &config.Config{
		Env:    "development",
		Cookie: config.CookieConfig{Domain: "capyrpi.org"},
	}}

	// Direct localhost
	rLocal := httptest.NewRequest("GET", "/", nil)
	rLocal.Host = "localhost:3000"
	assert.Equal(t, "localhost", h.getCookieDomain(rLocal))

	// Proxied to dev.capyrpi.org
	rProxied := httptest.NewRequest("GET", "/", nil)
	rProxied.Host = "dev.capyrpi.org"
	rProxied.Header.Set("X-Forwarded-Host", "localhost:3000")
	assert.Equal(t, "localhost", h.getCookieDomain(rProxied))

	// Production (not in dev mode)
	hProd := &Handler{Config: &config.Config{
		Env:    "production",
		Cookie: config.CookieConfig{Domain: "capyrpi.org"},
	}}
	rProd := httptest.NewRequest("GET", "/", nil)
	rProd.Host = "api.capyrpi.org"
	assert.Equal(t, "capyrpi.org", hProd.getCookieDomain(rProd))
}

func TestGetBaseURL(t *testing.T) {
	h := &Handler{Config: &config.Config{
		Env: "development",
	}}

	// Direct localhost
	rLocal := httptest.NewRequest("GET", "/", nil)
	rLocal.Host = "localhost:3000"
	assert.Equal(t, "http://localhost:3000", h.getBaseURL(rLocal))

	// Proxied with HTTPS
	rProxied := httptest.NewRequest("GET", "/", nil)
	rProxied.Host = "dev.capyrpi.org"
	rProxied.Header.Set("X-Forwarded-Host", "localhost:3000")
	rProxied.Header.Set("X-Forwarded-Proto", "https")
	assert.Equal(t, "https://localhost:3000", h.getBaseURL(rProxied))
}

func TestGetOAuthRedirectURL(t *testing.T) {
	h := &Handler{Config: &config.Config{
		Env: "development",
	}}

	providerURL := "https://api.capyrpi.org/api/v1/auth/google/callback"

	// Direct localhost
	rLocal := httptest.NewRequest("GET", "/", nil)
	rLocal.Host = "localhost:3000"
	assert.Equal(t, "http://localhost:3000/api/v1/auth/google/callback", h.getOAuthRedirectURL(rLocal, providerURL))

	// Proxied
	rProxied := httptest.NewRequest("GET", "/", nil)
	rProxied.Host = "dev.capyrpi.org"
	rProxied.Header.Set("X-Forwarded-Host", "localhost:3000")
	assert.Equal(t, "http://localhost:3000/api/v1/auth/google/callback", h.getOAuthRedirectURL(rProxied, providerURL))

	// Production env with dev host (should work now)
	hDevHost := &Handler{Config: &config.Config{Env: "production"}}
	rDevHost := httptest.NewRequest("GET", "/", nil)
	rDevHost.Host = "dev.capyrpi.org"
	rDevHost.Header.Set("X-Forwarded-Host", "localhost:3000")
	assert.Equal(t, "http://localhost:3000/api/v1/auth/google/callback", hDevHost.getOAuthRedirectURL(rDevHost, providerURL))

	// Production env with X-Dev-Host (bypassing proxy)
	rXDev := httptest.NewRequest("GET", "/", nil)
	rXDev.Host = "api.capyrpi.org"
	rXDev.Header.Set("X-Dev-Host", "localhost:5173")
	assert.Equal(t, "http://localhost:5173/api/v1/auth/google/callback", hDevHost.getOAuthRedirectURL(rXDev, providerURL))

	// Production env with X-Dev-Proto
	rXProto := httptest.NewRequest("GET", "/", nil)
	rXProto.Host = "api.capyrpi.org"
	rXProto.Header.Set("X-Dev-Host", "localhost:5173")
	rXProto.Header.Set("X-Dev-Proto", "https")
	assert.Equal(t, "https://localhost:5173/api/v1/auth/google/callback", hDevHost.getOAuthRedirectURL(rXProto, providerURL))

	// Production (not in dev mode and not a dev host)
	hProd := &Handler{Config: &config.Config{Env: "production"}}
	rProd := httptest.NewRequest("GET", "/", nil)
	rProd.Host = "api.capyrpi.org"
	assert.Equal(t, "", hProd.getOAuthRedirectURL(rProd, providerURL))
}
