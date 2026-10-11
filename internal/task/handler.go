package task

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/otyang/demo/internal/middleware"
	"github.com/otyang/demo/pkg/httpx"
	"github.com/otyang/demo/pkg/response"
)

const (
	// PermissionRead is required to get and list tasks.
	PermissionRead = "tasks:read"
	// PermissionWrite is required to create, update and delete tasks.
	PermissionWrite = "tasks:write"
)

// Handler serves the task routes.
type Handler struct {
	service *Service
}

// NewHandler returns the task routes for the caller to mount, at
// /api/v1/tasks. authenticate must put JWTClaims in the request context:
// middleware.JWTAuthentication, or middleware.TestClaims locally. Every
// route then requires an organisation, because tasks belong to the caller's
// organisation, and the matching read or write permission.
//
// Document any route added here in doc/openapi.yaml.
func NewHandler(service *Service, authenticate func(http.Handler) http.Handler) (http.Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("task handler: service cannot be nil")
	}
	if authenticate == nil {
		return nil, fmt.Errorf("task handler: authentication middleware cannot be nil")
	}
	h := &Handler{service: service}

	canRead := middleware.RequirePermission(PermissionRead)
	canWrite := middleware.RequirePermission(PermissionWrite)

	router := chi.NewRouter()
	router.Use(authenticate, middleware.RequireOrganization)

	router.With(canRead).Get("/", httpx.Handle(h.list))
	router.With(canWrite).Post("/", httpx.Handle(h.create))
	router.Route("/{id}", func(r chi.Router) {
		r.With(canRead).Get("/", httpx.Handle(h.get))
		r.With(canWrite).Patch("/", httpx.Handle(h.update))
		r.With(canWrite).Delete("/", httpx.Handle(h.delete))
	})
	return router, nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	var filter ListFilter
	// Any query key besides done, limit and after answers 400, so a
	// misspelt filter is not silently ignored.
	if err := response.BindQueryStrict(r, &filter, "limit", "after"); err != nil {
		return err
	}
	page, err := httpx.PageParams(r)
	if err != nil {
		return err
	}
	result, err := h.service.List(r.Context(), organisationID(r), filter, page)
	if err != nil {
		return err
	}
	return response.WriteListCursor(w, http.StatusOK, result.Data, result.Meta)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	var input CreateInput
	if err := response.Bind(r, &input); err != nil {
		return err
	}
	task, err := h.service.Create(r.Context(), organisationID(r), input)
	if err != nil {
		return err
	}
	return response.WriteSuccess(w, http.StatusCreated, task)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	task, err := h.service.Get(r.Context(), organisationID(r), chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	return response.WriteSuccess(w, http.StatusOK, task)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	var input UpdateInput
	if err := response.Bind(r, &input); err != nil {
		return err
	}
	task, err := h.service.Update(r.Context(), organisationID(r), chi.URLParam(r, "id"), input)
	if err != nil {
		return err
	}
	return response.WriteSuccess(w, http.StatusOK, task)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	if err := h.service.Delete(r.Context(), organisationID(r), chi.URLParam(r, "id")); err != nil {
		return err
	}
	return response.WriteJSON(w, http.StatusNoContent, nil)
}

// organisationID returns the caller's organisation. RequireOrganization
// guarantees the claims are present and name one.
func organisationID(r *http.Request) string {
	claims, _ := middleware.AuthClaimsFromContext(r.Context())
	return claims.OrganizationID
}
