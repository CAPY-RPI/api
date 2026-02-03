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
		expectedOrigin string
		expectedCreds  string
	}{
		{
			name:           "AllowedOrigin",
			origin:         "https://app.example.com",
			method:         "GET",
			setupOrigins:   allowedOrigins,
			expectedOrigin: "https://app.example.com",
			expectedCreds:  "true",
		},
		{
			name:           "DisallowedOrigin",
			origin:         "https://evil.com",
			method:         "GET",
			setupOrigins:   allowedOrigins,
			expectedOrigin: "",
			expectedCreds:  "",
		},
		{
			name:           "NoOriginHeader",
			origin:         "",
			method:         "GET",
			setupOrigins:   allowedOrigins,
			expectedOrigin: "",
			expectedCreds:  "",
		},
		{
			name:           "PreflightOptions",
			origin:         "https://app.example.com",
			method:         "OPTIONS",
			setupOrigins:   allowedOrigins,
			expectedOrigin: "https://app.example.com",
			expectedCreds:  "true",
		},
		{
			name:           "DevModeAllowAll",
			origin:         "https://random.com",
			method:         "GET",
			setupOrigins:   []string{}, // Empty = allow all
			expectedOrigin: "https://random.com",
			expectedCreds:  "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := middleware.CORS(tt.setupOrigins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(tt.method, "/", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
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
