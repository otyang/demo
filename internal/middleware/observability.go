package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/otyang/demo/pkg/response"
)

func Observability(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			writer := &statusWriter{ResponseWriter: w}
			defer func() {
				if recovered := recover(); recovered != nil {
					// http.ErrAbortHandler deliberately aborts the response;
					// net/http handles it quietly, so let it through.
					if recovered == http.ErrAbortHandler {
						panic(recovered)
					}
					logger.ErrorContext(r.Context(), "panic handling HTTP request",
						"panic", recovered,
						"stack", string(debug.Stack()),
						"request_id", requestID(r),
						"method", r.Method,
						"path", r.URL.Path,
					)
					if !writer.wroteHeader {
						if err := response.WriteErrorFrom(writer, response.NewInternalServerError(nil)); err != nil {
							logger.ErrorContext(r.Context(), "write panic response failed", "error", err)
						}
					}
				}

				status := writer.status
				if status == 0 {
					status = http.StatusOK
				}
				logger.InfoContext(r.Context(), "HTTP request",
					"request_id", requestID(r),
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"duration", time.Since(started),
				)
			}()

			next.ServeHTTP(writer, r)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// requestID returns the ID set by chi's RequestID middleware, or "" if unset.
func requestID(r *http.Request) string {
	return chimiddleware.GetReqID(r.Context())
}
