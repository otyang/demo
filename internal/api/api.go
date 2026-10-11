// Package api assembles the API: it wires each service's repository,
// service and handler, and mounts their routes on one router.
//
// Every route served here must be documented in doc/openapi.yaml, and every
// documented route served here; this package's tests check both.
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/otyang/demo/internal/bootstrap"
	"github.com/otyang/demo/internal/task"
)

// NewRouter migrates each service's tables and returns the router serving
// every route of the API.
func NewRouter(ctx context.Context, infra *bootstrap.Infrastructure) (*chi.Mux, error) {
	router := bootstrap.NewRouter(infra.Logger, infra.Config.CORSAllowedOrigins)

	tasks, err := newTaskHandler(ctx, infra)
	if err != nil {
		return nil, err
	}
	router.Mount("/api/v1/tasks", tasks)

	return router, nil
}

// newTaskHandler wires the task service: repository, then service, then
// handler.
func newTaskHandler(ctx context.Context, infra *bootstrap.Infrastructure) (http.Handler, error) {
	if err := task.Migrate(ctx, infra.Store.BunDB, infra.Logger); err != nil {
		return nil, fmt.Errorf("migrate tasks: %w", err)
	}
	repo, err := task.NewBunRepository(infra.Store.BunDB)
	if err != nil {
		return nil, err
	}
	service, err := task.NewService(repo)
	if err != nil {
		return nil, err
	}
	return task.NewHandler(service, infra.Authenticate)
}
