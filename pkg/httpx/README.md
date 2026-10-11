# HTTP handlers

`github.com/otyang/demo/pkg/httpx` provides `HandlerFunc`, an HTTP handler
that returns its error instead of writing it.

```go
type HandlerFunc func(http.ResponseWriter, *http.Request) error
```

`HandlerFunc` implements `http.Handler`. If the function returns an error,
`ServeHTTP` logs it with `slog.Default()` and writes the shared
`pkg/response` error envelope. Errors that are not `response.ResponseError`
become 500 responses with a generic message; the original error appears only
in the log. Each error is logged once as `request failed`, with the request ID,
method, path, status, and reason. 4xx errors log at Warn and 5xx at Error.

Two cases are logged but not written:

- **The response had already started.** Writing an error envelope on top of
  it would corrupt the response, so the error is logged as
  `request failed after the response started`.
- **The client cancelled the request.** An error wrapping `context.Canceled`
  while the request's context is done is logged at Info as
  `request cancelled by client`, because nobody is left to read a response.

## Unknown routes and methods

`httpx.NotFound` and `httpx.MethodNotAllowed` answer unknown routes (404) and
wrong methods (405) with the same envelope. Register them on the top-level
router; chi passes them on to mounted subrouters:

```go
router.NotFound(httpx.NotFound)
router.MethodNotAllowed(httpx.MethodNotAllowed)
```

## Route handlers

```go
func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	thing, err := h.service.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	return response.WriteSuccess(w, http.StatusOK, thing)
}

router.Get("/{id}", httpx.Handle(h.get))
```

`httpx.Handle` returns the `http.HandlerFunc` that chi's `Get`, `Post`, `Put`,
`Patch` and `Delete` expect, so handler methods can be passed directly. Where
an `http.Handler` is expected (for example `router.Method` or `Mount`), use
`httpx.HandlerFunc(h.get)` instead.

## Middleware

Middleware keeps the standard `func(http.Handler) http.Handler` shape, so it
works with chi's `Use` (every route) and `With` (one route or group), and with
third-party middleware such as chi's `RequestID`. Use `httpx.HandlerFunc` as
the body so the middleware can return errors.

Inside the body, choose one of two outcomes:

- **Stop the request:** return an error. It is logged and written for you,
  and the next handler never runs.
- **Continue:** call `next.ServeHTTP(w, r)`, then `return nil`.

Never do both, and never call `next` more than once.

### A middleware with no settings

The function itself is the middleware:

```go
// RequireJSON rejects requests that send a body without a JSON content type.
func RequireJSON(next http.Handler) http.Handler {
	return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		if r.ContentLength > 0 && r.Header.Get("Content-Type") != "application/json" {
			return response.NewError(
				http.StatusUnsupportedMediaType,
				"UNSUPPORTED_MEDIA_TYPE",
				"Content-Type must be application/json",
				nil,
				nil,
			)
		}
		next.ServeHTTP(w, r)
		return nil
	})
}

router.Use(RequireJSON) // pass the function itself, without calling it
```

### A middleware with settings

Wrap the middleware in a function that takes the settings and returns it.
`middleware.RequirePermission` works this way:

```go
// RequireHeader rejects requests that are missing the named header.
func RequireHeader(name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			if r.Header.Get(name) == "" {
				return response.NewBadRequestError(name+" header is required", nil, nil)
			}
			next.ServeHTTP(w, r)
			return nil
		})
	}
}

router.Use(RequireHeader("X-Client-ID")) // call it to get the middleware
```

### Passing a value to handlers

Store the value in the request context under an unexported key type, and
provide a function that reads it back. `middleware.JWTAuthentication` and
`middleware.AuthClaimsFromContext` use this pattern:

```go
type clientIDKey struct{}

// ClientID stores the X-Client-ID header for handlers further down the chain.
func ClientID(next http.Handler) http.Handler {
	return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		id := r.Header.Get("X-Client-ID")
		if id == "" {
			return response.NewBadRequestError("X-Client-ID header is required", nil, nil)
		}
		ctx := context.WithValue(r.Context(), clientIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx)) // pass the new request on
		return nil
	})
}

// ClientIDFrom returns the client ID stored by ClientID.
func ClientIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(clientIDKey{}).(string)
	return id, ok
}

// In a handler:
func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	clientID, _ := ClientIDFrom(r.Context())
	// ...
}
```

### Doing work after the handler

Code after `next.ServeHTTP` runs once the handler has finished. Set response
headers *before* calling `next`: once the handler writes the response, header
changes are ignored.

```go
// WarnSlowRequests logs requests that take longer than limit.
func WarnSlowRequests(limit time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-Slow-Threshold", limit.String()) // before next

			started := time.Now()
			next.ServeHTTP(w, r)

			if elapsed := time.Since(started); elapsed > limit { // after next
				slog.WarnContext(r.Context(), "slow request",
					"path", r.URL.Path, "duration", elapsed)
			}
			return nil
		})
	}
}
```

### A middleware that never fails

A middleware that never rejects a request has no error to return, so a plain
`http.HandlerFunc` is enough:

```go
// NoCache stops clients and proxies from caching responses.
func NoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
```

## Using middleware

```go
router := chi.NewRouter()

// Every route on this router. Use must come before any route is registered.
router.Use(chimiddleware.RequestID, NoCache, RequireJSON)

// One route only.
router.With(RequireHeader("X-Client-ID")).Get("/reports", httpx.Handle(h.reports))

// Several middlewares on one route, run left to right.
router.With(ClientID, WarnSlowRequests(time.Second)).Post("/imports", httpx.Handle(h.importCSV))

// A group of routes sharing middleware, without a path prefix.
router.Group(func(r chi.Router) {
	r.Use(ClientID)
	r.Get("/usage", httpx.Handle(h.usage))
	r.Get("/limits", httpx.Handle(h.limits))
})

// A subrouter under a path prefix: /{id}, /{id}/verify, and so on.
router.Route("/{id}", func(r chi.Router) {
	r.Use(ClientID)
	r.Get("/", httpx.Handle(h.get))
	r.With(RequireHeader("X-Confirm")).Post("/verify", httpx.Handle(h.verify))
})
```

### Run order

Middlewares run in the order they are listed. Those from `Use` run first, then
those from `With`, then the handler. Code after `next.ServeHTTP` runs on the
way back, in reverse order:

```text
router.Use(A, B)
router.With(C).Get("/", httpx.Handle(h.get))

request  →  A  →  B  →  C  →  h.get
response ←  A  ←  B  ←  C  ←
```

If `B` returns an error, `C` and `h.get` never run, and `A`'s code after
`next.ServeHTTP` still runs.

## Testing a middleware

Wrap a stub handler and send requests through it with `httptest`:

```go
func TestRequireHeader(t *testing.T) {
	handler := RequireHeader("X-Client-ID")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusBadRequest, missing.Code)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Client-ID", "client-1")
	present := httptest.NewRecorder()
	handler.ServeHTTP(present, request)
	require.Equal(t, http.StatusNoContent, present.Code)
}
```

## Common mistakes

| Mistake | What happens | Fix |
|---|---|---|
| Returning an error after calling `next` | The error response is written on top of the handler's response | Return the error *instead of* calling `next` |
| Forgetting to call `next` | The request stops with an empty 200 response | Call `next.ServeHTTP(w, r)` when the request should continue |
| Calling `next` with the old request after adding a context value | Handlers never see the value | Pass `r.WithContext(ctx)` to `next` |
| Setting a header after `next` | The header is silently dropped | Set headers before calling `next` |
| Logging the error yourself before returning it | The error is logged twice | Just return it; `httpx` logs it |
| `router.Use(RequireHeader)` without arguments | Compile error | Call it: `RequireHeader("X-Client-ID")` |
