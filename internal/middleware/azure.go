package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// AzureHeaders adds standard Azure response headers to every response.
func AzureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-ms-request-id", uuid.New().String())
		w.Header().Set("x-ms-correlation-request-id", uuid.New().String())
		w.Header().Set("x-ms-routing-request-id", "AZEMU:"+uuid.New().String())
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// RequireAPIVersion rejects ARM-style calls that lack ?api-version=,
// except for metadata and auth endpoints which don't require it.
func RequireAPIVersion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Metadata and auth endpoints don't require api-version
		if strings.HasPrefix(path, "/metadata") ||
			strings.Contains(path, "/oauth2") ||
			strings.Contains(path, "/.well-known/") ||
			strings.Contains(path, "/discovery/") {
			next.ServeHTTP(w, r)
			return
		}

		// ARM endpoints require api-version
		if strings.HasPrefix(path, "/subscriptions") && r.URL.Query().Get("api-version") == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    "MissingApiVersionParameter",
					"message": "The api-version query parameter is required for all ARM requests.",
				},
			}); err != nil {
				log.Error().Err(err).Str("path", path).Msg("failed to write error response")
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}
