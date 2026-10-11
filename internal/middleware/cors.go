package middleware

import (
	"net/http"

	"github.com/go-chi/cors"
)

var predefinedOrigins = []string{
	"*",
	"http://localhost:3000",
}

// CORS wraps an HTTP handler with cross-origin resource sharing policy.
// allowedOrigins are added to the predefined origins below. It is the front
// office's policy.
func CORS(allowedOrigins ...string) func(http.Handler) http.Handler {
	return corsHandler(append(append([]string(nil), predefinedOrigins...), allowedOrigins...))
}

// CORSAllowlist allows exactly allowedOrigins, with no predefined ones and
// no wildcard: the back office's policy. With none, it adds no CORS headers
// at all, so browsers refuse cross-origin calls (an empty list must not reach
// the cors package, which treats it as "allow every origin").
func CORSAllowlist(allowedOrigins ...string) func(http.Handler) http.Handler {
	var origins []string
	for _, origin := range allowedOrigins {
		if origin != "" && origin != "*" {
			origins = append(origins, origin)
		}
	}
	if len(origins) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return corsHandler(origins)
}

func corsHandler(origins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins: origins,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-Request-ID",
		},
		// Let browser clients read the request ID to quote in bug reports.
		ExposedHeaders: []string{RequestIDHeader},
		MaxAge:         86400, // 24 hours
	})
}
