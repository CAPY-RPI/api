package middleware

import "net/http"

// CORS adds CORS headers for cross-origin requests
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := false

			// Allow if origin is in allowlist
			for _, o := range allowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}

			// If no allowed origins specified, allow all (development mode)
			// But for credentials to work with *, strict browsers block it.
			// So good practice: if development, echo back origin.
			if len(allowedOrigins) == 0 {
				allowed = true
			}

			if allowed && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Bot-Token, X-API-Key")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
