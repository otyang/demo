package bootstrap

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/otyang/demo/internal/middleware"
	"github.com/otyang/demo/pkg/httpx"
	"github.com/otyang/demo/pkg/response"
	"github.com/otyang/demo/pkg/version"
)

// NewRouter returns the API's root router with the middleware every route
// shares: a request ID, request logging with panic recovery, and CORS for
// corsOrigins only. It serves GET /healthz, and answers unknown routes and
// methods with the shared error envelope. Services mount their routes on it.
func NewRouter(logger *slog.Logger, corsOrigins []string) *chi.Mux {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Observability(logger))
	router.Use(middleware.CORSAllowlist(corsOrigins...))
	router.NotFound(httpx.NotFound)
	router.MethodNotAllowed(httpx.MethodNotAllowed)
	router.Get("/healthz", httpx.Handle(healthz))
	return router
}

type health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// healthz reports that the server is up, with its version. It is a
// liveness check: it does not check the database.
func healthz(w http.ResponseWriter, _ *http.Request) error {
	return response.WriteSuccess(w, http.StatusOK, health{Status: "ok", Version: version.Value})
}
