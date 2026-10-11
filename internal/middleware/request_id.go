package middleware

import (
	"context"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/otyang/demo/pkg/ids"
)

// RequestIDHeader carries the request ID back to the client so it can be
// quoted when reporting a problem.
const RequestIDHeader = "X-Request-ID"

// RequestID gives every request a fresh server-generated ID, ignoring any ID
// the client sent, so clients cannot choose the IDs that appear in logs. The
// ID is stored where chi's middleware.GetReqID finds it, and is returned in
// the X-Request-ID response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ids.New("req_")
		w.Header().Set(RequestIDHeader, id)
		ctx := context.WithValue(r.Context(), chimiddleware.RequestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
