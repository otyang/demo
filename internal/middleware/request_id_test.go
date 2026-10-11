package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/require"
)

func TestRequestIDIgnoresClientValueAndIsReturned(t *testing.T) {
	var seen string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = chimiddleware.GetReqID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-Id", "attacker-chosen-id")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	require.True(t, strings.HasPrefix(seen, "req_"), "request ID %q should be server-generated", seen)
	require.NotEqual(t, "attacker-chosen-id", seen)
	require.Equal(t, seen, recorder.Header().Get(RequestIDHeader))

	first := seen
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/", nil))
	require.NotEqual(t, first, second.Header().Get(RequestIDHeader), "each request needs its own ID")
}
