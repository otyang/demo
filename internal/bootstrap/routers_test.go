package bootstrap

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otyang/demo/internal/middleware"
)

func do(handler http.Handler, method, path string, header ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	for i := 0; i+1 < len(header); i += 2 {
		request.Header.Set(header[i], header[i+1])
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestHealth(t *testing.T) {
	recorder := do(NewRouter(slog.New(slog.DiscardHandler), nil), http.MethodGet, "/healthz")
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.NotEmpty(t, recorder.Header().Get(middleware.RequestIDHeader))

	var body struct {
		Status bool   `json:"status"`
		Data   health `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.True(t, body.Status)
	assert.Equal(t, "ok", body.Data.Status)
}

func TestUnknownRoutesUseTheErrorEnvelope(t *testing.T) {
	router := NewRouter(slog.New(slog.DiscardHandler), nil)
	notFound := do(router, http.MethodGet, "/nope")
	assert.Equal(t, http.StatusNotFound, notFound.Code)
	assert.JSONEq(t, `{"status":false,"error":{"code":"NOT_FOUND","message":"Route not found"}}`, notFound.Body.String())

	assert.Equal(t, http.StatusMethodNotAllowed, do(router, http.MethodPost, "/healthz").Code)
}

func TestCORSAllowsOnlyTheConfiguredOrigins(t *testing.T) {
	router := NewRouter(slog.New(slog.DiscardHandler), []string{"http://localhost:8080"})
	allowed := do(router, http.MethodGet, "/healthz", "Origin", "http://localhost:8080")
	assert.Equal(t, "http://localhost:8080", allowed.Header().Get("Access-Control-Allow-Origin"))
	other := do(router, http.MethodGet, "/healthz", "Origin", "http://evil.test")
	assert.Empty(t, other.Header().Get("Access-Control-Allow-Origin"))
}
