package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/otyang/demo/internal/bootstrap"
	"github.com/otyang/demo/internal/middleware"
	"github.com/otyang/demo/pkg/store"
)

// specFile is the API reference served by the doc command.
const specFile = "../../doc/openapi.yaml"

func newTestRouter(t *testing.T) *chi.Mux {
	t.Helper()
	db, err := store.New(store.Options{
		Type: store.SQLite, DSN: "file:" + t.Name() + "?mode=memory&cache=shared", MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	router, err := NewRouter(context.Background(), &bootstrap.Infrastructure{
		Config:       &bootstrap.AppConfig{},
		Logger:       slog.New(slog.DiscardHandler),
		Store:        db,
		Authenticate: middleware.TestClaims(middleware.JWTClaims{}),
	})
	require.NoError(t, err)
	return router
}

// served returns "METHOD /path" for every route the router serves.
func served(t *testing.T, router chi.Routes) []string {
	t.Helper()
	var routes []string
	require.NoError(t, chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if route != "/" {
			route = strings.TrimSuffix(route, "/") // a mounted router's root, such as /api/v1/tasks/
		}
		routes = append(routes, method+" "+route)
		return nil
	}))
	slices.Sort(routes)
	return routes
}

// documented returns "METHOD /path" for every operation in the spec.
func documented(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(specFile)
	require.NoError(t, err)
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &spec))

	methods := []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}
	var routes []string
	for path, item := range spec.Paths {
		for key := range item {
			if slices.Contains(methods, key) { // skip path-level keys such as parameters
				routes = append(routes, strings.ToUpper(key)+" "+path)
			}
		}
	}
	slices.Sort(routes)
	return routes
}

// TestSpecMatchesTheServedRoutes fails when a route is added, removed or
// renamed without updating doc/openapi.yaml.
func TestSpecMatchesTheServedRoutes(t *testing.T) {
	assert.Equal(t, served(t, newTestRouter(t)), documented(t),
		"the routes served (expected) and documented in %s (actual) differ", specFile)
}

func TestNewRouterServesEveryService(t *testing.T) {
	routes := served(t, newTestRouter(t))
	assert.Contains(t, routes, "GET /healthz")
	assert.Contains(t, routes, "POST /api/v1/tasks")
}
