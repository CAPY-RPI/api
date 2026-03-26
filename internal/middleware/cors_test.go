package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/capyrpi/api/internal/middleware"
	"github.com/stretchr/testify/assert"
)

func TestCORS(t *testing.T) {
	allowedOrigins := []string{"https://app.example.com", "http://localhost:3000"}

	tests := []struct {
		name           string
		origin         string
		method         string
		setupOrigins   []string
		isDev          bool
		expectedOrigin string
		expectedCreds  string
		forwardedHost  string
	}{
		{
			name:           "AllowedOrigin",
			origin:         "https://app.example.com",
			method:         "GET",
			setupOrigins:   allowedOrigins,
			isDev:          false,
			expectedOrigin: "https://app.example.com",
			expectedCreds:  "true",
		},
		{
			name:           "DisallowedOrigin",
			origin:         "https://evil.com",
			method:         "GET",
			setupOrigins:   allowedOrigins,
			isDev:          false,
			expectedOrigin: "",
			expectedCreds:  "",
		},
		{
			name:           "NoOriginHeader",
			origin:         "",
			method:         "GET",
			setupOrigins:   allowedOrigins,
			isDev:          false,
			expectedOrigin: "",
			expectedCreds:  "",
		},
		{
			name:           "PreflightOptions",
			origin:         "https://app.example.com",
			method:         "OPTIONS",
			setupOrigins:   allowedOrigins,
			isDev:          false,
			expectedOrigin: "https://app.example.com",
			expectedCreds:  "true",
		},
		{
			name:           "DevModeAllowAll",
			origin:         "https://random.com",
			method:         "GET",
			setupOrigins:   []string{}, // Empty = allow all in dev
			isDev:          true,
			expectedOrigin: "https://random.com",
			expectedCreds:  "true",
		},
		{
			name:           "AllowedLocalhostInDev",
			origin:         "http://localhost:9999",
			method:         "GET",
			setupOrigins:   []string{"https://app.example.com"},
			isDev:          true,
			expectedOrigin: "http://localhost:9999",
			expectedCreds:  "true",
		},
		{
			name:           "AllowedForwardedHostInDev",
			origin:         "https://my-frontend.local",
			method:         "GET",
			setupOrigins:   []string{"https://app.example.com"},
			isDev:          true,
			forwardedHost:  "my-frontend.local",
			expectedOrigin: "https://my-frontend.local",
			expectedCreds:  "true",
		},
		{
			name:           "BlockedLocalhostInProd",
			origin:         "http://localhost:3000",
			method:         "GET",
			setupOrigins:   []string{"http://localhost:3000", "https://app.example.com"},
			isDev:          false,
			expectedOrigin: "",
			expectedCreds:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := middleware.CORS(tt.setupOrigins, tt.isDev)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(tt.method, "/", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedOrigin, rr.Header().Get("Access-Control-Allow-Origin"))
			assert.Equal(t, tt.expectedCreds, rr.Header().Get("Access-Control-Allow-Credentials"))

			if tt.method == "OPTIONS" && tt.expectedOrigin != "" {
				assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", rr.Header().Get("Access-Control-Allow-Methods"))
			}
		})
	}
}
