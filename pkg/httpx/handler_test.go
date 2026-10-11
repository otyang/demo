package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/otyang/demo/pkg/response"
)

func TestHandlerFuncLogsWithDefaultLoggerAndWritesError(t *testing.T) {
	logs := captureLogs(t)

	handler := HandlerFunc(func(http.ResponseWriter, *http.Request) error {
		return errors.New("database exploded")
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/things", nil))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "database exploded", "internal details must not reach the client")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	require.Equal(t, "ERROR", entry["level"])
	require.Equal(t, "request failed", entry["msg"])
	require.Equal(t, "/things", entry["path"])
	require.Equal(t, float64(http.StatusInternalServerError), entry["status"])
	require.Contains(t, entry["error"], "database exploded")
}

func TestHandlerFuncDoesNothingOnSuccess(t *testing.T) {
	logs := captureLogs(t)

	handler := HandlerFunc(func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Empty(t, logs.String())
}

// TestHandlerFuncWorksAsRouteAndMiddleware registers a HandlerFunc-based
// middleware globally (Use) and per route (With), and a HandlerFunc as the
// route itself. Each error must be logged and written exactly once.
func TestHandlerFuncWorksAsRouteAndMiddleware(t *testing.T) {
	logs := captureLogs(t)

	requireHeader := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
				if r.Header.Get(name) == "" {
					return response.NewBadRequestError(name+" header is required", nil, nil)
				}
				next.ServeHTTP(w, r)
				return nil
			})
		}
	}

	router := chi.NewRouter()
	router.Use(requireHeader("X-Global"))
	router.With(requireHeader("X-Route")).Method(http.MethodGet, "/", HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		if r.URL.Query().Get("fail") != "" {
			return response.NewNotFoundError("Thing not found", nil)
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))

	for _, test := range []struct {
		name    string
		headers []string
		query   string
		status  int
		message string
	}{
		{name: "global middleware rejects", status: http.StatusBadRequest, message: "X-Global header is required"},
		{name: "route middleware rejects", headers: []string{"X-Global"}, status: http.StatusBadRequest, message: "X-Route header is required"},
		{name: "handler returns error", headers: []string{"X-Global", "X-Route"}, query: "?fail=1", status: http.StatusNotFound, message: "Thing not found"},
		{name: "success", headers: []string{"X-Global", "X-Route"}, status: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			logs.Reset()
			request := httptest.NewRequest(http.MethodGet, "/"+test.query, nil)
			for _, header := range test.headers {
				request.Header.Set(header, "1")
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			require.Equal(t, test.status, recorder.Code)
			if test.message == "" {
				require.Empty(t, logs.String())
				return
			}
			lines := bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n"))
			require.Len(t, lines, 1, "error must be logged exactly once")
			var entry map[string]any
			require.NoError(t, json.Unmarshal(lines[0], &entry))
			require.Equal(t, "WARN", entry["level"])
			require.Equal(t, test.message, entry["error_message"])
			require.Contains(t, recorder.Body.String(), test.message)
		})
	}
}

func TestHandleWorksWithChiShortcuts(t *testing.T) {
	logs := captureLogs(t)

	router := chi.NewRouter()
	router.Get("/things/{id}", Handle(func(w http.ResponseWriter, r *http.Request) error {
		if chi.URLParam(r, "id") == "missing" {
			return response.NewNotFoundError("Thing not found", nil)
		}
		return response.WriteSuccess(w, http.StatusOK, chi.URLParam(r, "id"))
	}))

	found := httptest.NewRecorder()
	router.ServeHTTP(found, httptest.NewRequest(http.MethodGet, "/things/42", nil))
	require.Equal(t, http.StatusOK, found.Code)
	require.Contains(t, found.Body.String(), `"42"`)
	require.Empty(t, logs.String())

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/things/missing", nil))
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Contains(t, logs.String(), "Thing not found")
}

func TestErrorAfterResponseStartedIsLoggedNotWritten(t *testing.T) {
	logs := captureLogs(t)
	handler := HandlerFunc(func(w http.ResponseWriter, _ *http.Request) error {
		if err := response.WriteSuccess(w, http.StatusOK, "partial"); err != nil {
			return err
		}
		return errors.New("failed after writing")
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "INTERNAL", "no second response may be written")
	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	require.Equal(t, "request failed after the response started", entry["msg"])
	require.Equal(t, "ERROR", entry["level"])
}

func TestClientCancellationIsNotAnError(t *testing.T) {
	logs := captureLogs(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the client has gone away
	handler := HandlerFunc(func(_ http.ResponseWriter, r *http.Request) error {
		return response.NewDatabaseError(fmt.Errorf("query: %w", r.Context().Err()))
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))

	require.Zero(t, recorder.Body.Len(), "nothing should be written to a cancelled client")
	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	require.Equal(t, "INFO", entry["level"])
	require.Equal(t, "request cancelled by client", entry["msg"])
}

func TestNotFoundAndMethodNotAllowedUseEnvelope(t *testing.T) {
	captureLogs(t)
	router := chi.NewRouter()
	router.NotFound(NotFound)
	router.MethodNotAllowed(MethodNotAllowed)
	sub := chi.NewRouter()
	sub.Get("/{id}", Handle(func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
	router.Mount("/things", sub)

	for _, test := range []struct {
		method, path string
		status       int
		code         string
	}{
		{http.MethodGet, "/missing", http.StatusNotFound, "NOT_FOUND"},
		{http.MethodGet, "/things/1/extra", http.StatusNotFound, "NOT_FOUND"},
		{http.MethodDelete, "/things/1", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
		require.Equal(t, test.status, recorder.Code, "%s %s", test.method, test.path)
		var envelope struct {
			Status bool `json:"status"`
			Error  struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope), "%s %s body %q", test.method, test.path, recorder.Body.String())
		require.False(t, envelope.Status)
		require.Equal(t, test.code, envelope.Error.Code)
	}
}

// captureLogs sends slog.Default() and the standard log package to a buffer
// for the rest of the test, restoring both afterwards. slog.SetDefault also
// redirects the log package, so restoring slog alone would leave log writing
// into this test's buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous, previousWriter, previousFlags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})
	return &logs
}
