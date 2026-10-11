# bootstrap

Starts the application: configuration, shared infrastructure, the router,
and the HTTP server.

## Infrastructure

```go
infra, err := bootstrap.NewInfrastructure(ctx, ".env")
defer infra.Close()
```

`NewInfrastructure` loads `AppConfig` from the env file and `DEMO_*`
variables (with `pkg/config`), checks it, and opens:

- `infra.Logger`: JSON logs, or colored text when `DEMO_DEBUG=true`
  (`pkg/logger`). It is also made `slog.Default()`, which `httpx` logs
  request errors with.
- `infra.Store`: the SQLite or PostgreSQL database (`pkg/store`). SQLite
  always uses one connection, as it allows one writer at a time.
- `infra.Authenticate`: `middleware.JWTAuthentication` with a JWKS verifier
  (`pkg/jwtutils`), or `middleware.TestClaims` when
  `DEMO_AUTH_TEST_CLAIMS=true`.

Startup refuses unsafe settings: test claims without `DEMO_DEBUG=true`, JWT
verification without a JWKS URL, issuer and audience, or a PostgreSQL pool
smaller than 2 (migrations hold one connection for their lock).

## Router

```go
router := bootstrap.NewRouter(infra.Logger, infra.Config.CORSAllowedOrigins)
router.Mount("/api/v1/tasks", tasks)
```

`NewRouter` adds the middleware every route shares (request ID, logging and
panic recovery, CORS allowlist), serves `GET /healthz`, and answers unknown
routes and methods with the error envelope. Services mount their own
routers on it; [internal/api](../api/api.go) does this for the whole API.

## Server

```go
err := bootstrap.Bootstrap(ctx, name, addr, router, infra.Logger)
```

Serves until `ctx` is cancelled, then shuts down gracefully, letting
running requests finish for up to 10 seconds. Read, write and idle timeouts
are set.
