// Package httpx provides an error-returning HTTP handler type that plugs into
// standard net/http routing and middleware.
package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/otyang/demo/pkg/response"
)

// HandlerFunc is an HTTP handler that returns its error instead of writing it.
// It implements http.Handler, so it can be registered on routes directly and
// used as the body of standard func(http.Handler) http.Handler middleware:
//
//	router.Method(http.MethodGet, "/", httpx.HandlerFunc(h.list))
//
// A returned error is logged with slog.Default() and written as the shared
// error envelope; errors that are not response.ResponseError become 500s.
type HandlerFunc func(http.ResponseWriter, *http.Request) error

func (h HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tracked := &responseTracker{ResponseWriter: w}
	if err := h(tracked, r); err != nil {
		writeError(tracked, r, err)
	}
}

// Handle returns h as the http.HandlerFunc that chi's Get, Post, Put, Patch
// and Delete expect. Error-returning functions and methods can be passed
// directly, without converting them to HandlerFunc first:
//
//	router.Get("/{id}", httpx.Handle(h.get))
func Handle(h HandlerFunc) http.HandlerFunc {
	return h.ServeHTTP
}

// NotFound answers requests for routes that do not exist with the shared
// error envelope. Register it with chi's router.NotFound.
func NotFound(w http.ResponseWriter, r *http.Request) {
	writeError(&responseTracker{ResponseWriter: w}, r, response.NewNotFoundError("Route not found", nil))
}

// MethodNotAllowed answers requests whose route exists but not for the
// method used, with the shared error envelope. Register it with chi's
// router.MethodNotAllowed.
func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeError(&responseTracker{ResponseWriter: w}, r, response.NewError(
		http.StatusMethodNotAllowed,
		"METHOD_NOT_ALLOWED",
		"Method not allowed",
		nil,
		nil,
	))
}

// writeError is the single place request errors are logged and written,
// whether they come from a route handler or from middleware. Client errors
// (4xx) log at Warn; server errors at Error.
//
// Two cases are logged but not written: the client cancelled the request
// (nobody is left to read a response), and the handler had already started
// its response (a second response would corrupt it).
func writeError(w *responseTracker, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
		slog.InfoContext(r.Context(), "request cancelled by client",
			"request_id", chimiddleware.GetReqID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"error", err,
		)
		return
	}

	appErr, ok := errors.AsType[response.ResponseError](err)
	if !ok {
		appErr = response.NewInternalServerError(err)
	}
	status := appErr.HTTPStatus()
	level := slog.LevelWarn
	if status >= http.StatusInternalServerError {
		level = slog.LevelError
	}
	message := "request failed"
	if w.wroteHeader {
		message = "request failed after the response started"
	}
	slog.Log(r.Context(), level, message,
		"request_id", chimiddleware.GetReqID(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"error_code", appErr.ErrorPayload().Code,
		"error_message", appErr.ErrorPayload().Message,
		"error", appErr,
	)
	if w.wroteHeader {
		return
	}

	if writeErr := response.WriteErrorFrom(w, appErr); writeErr != nil {
		slog.ErrorContext(r.Context(), "write error response failed", "error", writeErr)
	}
}

// responseTracker records whether a response has started, so writeError
// never writes a second response on top of one.
type responseTracker struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *responseTracker) WriteHeader(status int) {
	// 1xx responses (such as 103 Early Hints) precede the real response.
	if status >= http.StatusOK {
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseTracker) Write(body []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(body)
}

// Unwrap lets http.ResponseController reach the underlying writer, for
// example to flush or set deadlines.
func (w *responseTracker) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
