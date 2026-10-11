# demo

Written by **Young Otutu**.

A small base project for Go REST APIs. It is not a framework: it is a set of
plain packages (`pkg/` and `internal/middleware`) and one example service,
`internal/task`, that shows how they fit together. Copy the example to start
a new service.

## Quick start

Requires Go 1.27 or newer. Nothing else: the default database is a local
SQLite file.

```sh
go run ./cmd/api   # the API, on http://localhost:3000
go run ./doc       # its reference, on http://localhost:8080/docs
```

Or run the API with live reload: install [Air](https://github.com/air-verse/air)
once, then start it from the repository root. It rebuilds and restarts the
API whenever a Go file changes (see [.air.toml](.air.toml)).

```sh
go install github.com/air-verse/air@latest
air
```

Then try a request:

```sh
curl -X POST localhost:3000/api/v1/tasks \
  -H 'Content-Type: application/json' -d '{"title":"Try the demo"}'
curl localhost:3000/api/v1/tasks
```

Local mode needs no tokens: `.example.env` enables test claims, which give
every request a fixed organisation and the `tasks:read` and `tasks:write`
permissions. More requests are in [cmd/api/api.http](cmd/api/api.http) (VS
Code REST Client).

## How a request flows

```text
request
  → RequestID, Observability (logging, panic recovery), CORS   bootstrap.NewRouter
  → authenticate, RequireOrganization, RequirePermission        task.NewHandler
  → handler: bind the request, call the service, write the response
  → service: validate, apply the rules, map errors to HTTP errors
  → repository: read and write the database
```

Handlers return errors instead of writing them (`httpx.HandlerFunc`). Every
success is written in one envelope:

```json
{
  "status": true,
  "data": [
    {
      "id": "task_0b7e5a52-3c1f-4d8e-9a3b-2f6c1d9e8a10",
      "title": "Try the demo",
      "notes": "",
      "done": false,
      "createdAt": "2026-10-11T01:06:29Z",
      "updatedAt": "2026-10-11T01:06:29Z"
    }
  ],
  "meta": {
    "limit": 20,
    "hasMore": false
  }
}
```

and every error, including unknown routes, in another:

```json
{
  "status": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "details": [
      {
        "field": "title",
        "tag": "required",
        "message": "title is a required field"
      }
    ]
  }
}
```

## API reference

[doc/openapi.yaml](doc/openapi.yaml) documents every route as OpenAPI 3.1,
and `go run ./doc` serves it with Scalar. The tests in
[internal/api](internal/api/api_test.go) walk the API's router and fail if
the served routes and the documented ones differ, so the reference cannot
silently fall behind the code. See [doc](doc/README.md).

## Adding a service

1. Copy `internal/task` to `internal/<name>` and rename the model, inputs,
   table and permissions.
2. Write its migrations in `internal/<name>/migrations`.
3. In [internal/api/api.go](internal/api/api.go), wire the repository,
   service and handler, and mount the handler.
4. Document its routes in [doc/openapi.yaml](doc/openapi.yaml).

## Configuration

`cmd/api` loads `.env` when it exists, otherwise `.example.env` (or the file
given with `-env`). Variables already set in the environment win.

| Variable | Default | Meaning |
| --- | --- | --- |
| `DEMO_APP_NAME` | `demo-api` | Name in the logs |
| `DEMO_DEBUG` | `false` | Colored logs with source lines, and SQL query logs |
| `DEMO_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `DEMO_HOST` | `localhost:3000` | Listen address |
| `DEMO_CORS_ALLOWED_ORIGINS` | none | Browser origins allowed to call the API, comma-separated |
| `DEMO_DB_TYPE` | `sqlite` | `sqlite` or `postgres` |
| `DEMO_DB_URL` | required | `file:demo.db`, or `postgres://user:pass@host:5432/db` |
| `DEMO_DB_TIMEOUT` | `5s` | Bounds the startup connection check |
| `DEMO_DB_POOL_SIZE` | `10` | Connections (postgres: at least 2; sqlite always uses 1) |
| `DEMO_AUTH_TEST_CLAIMS` | `false` | Skip JWT checks and use fixed claims; requires `DEMO_DEBUG=true` |
| `DEMO_AUTH_TEST_ORG_ID` | `org_demo` | The test claims' organisation |
| `DEMO_AUTH_TEST_PERMISSIONS` | none | The test claims' permissions, comma-separated |
| `DEMO_AUTH_JWKS_URL` | | Signing keys of your identity provider |
| `DEMO_AUTH_ISSUER` | | Required `iss` of tokens |
| `DEMO_AUTH_AUDIENCE` | | Required `aud` of tokens (when they carry one) |

Without test claims, the three `DEMO_AUTH_*` URLs are required and every
protected route needs `Authorization: Bearer <JWT>` with an `org_id` claim
and a `permissions` array claim.

## Layout

```text
cmd/api/              Entrypoint: loads the configuration and serves the API
doc/                  openapi.yaml, and the command serving it at /docs
internal/
  api/                Wires every service and mounts its routes on one router
  bootstrap/          Configuration, infrastructure, root router, server lifecycle
  middleware/         JWT auth, permissions, organisation, CORS, request ID, logging
  task/               Example service: model, repository, service, handler, migrations
pkg/
  config/             Loads env files and variables into a struct
  httpx/              Error-returning handlers, 404/405, pagination query params
  ids/                Prefixed UUID identifiers
  jwtutils/           JWT verification against a JWKS
  logger/             slog setup (JSON or colored text)
  pagination/         Cursor pagination
  response/           JSON envelopes, typed HTTP errors, request binding
  store/              Bun database (SQLite or PostgreSQL) and migrations
  validation/         Struct validation with readable messages
  version/            Version string
```

Each package has its own README: [bootstrap](internal/bootstrap/README.md),
[task](internal/task/README.md), [doc](doc/README.md),
[config](pkg/config/README.md), [httpx](pkg/httpx/README.md),
[ids](pkg/ids/README.md), [logger](pkg/logger/README.md),
[pagination](pkg/pagination/README.md), [response](pkg/response/README.md),
[store](pkg/store/README.md), [validation](pkg/validation/README.md).

## Tests

```sh
go test ./...
```

The task tests run real HTTP requests through the router, middleware,
handler, service and repository against an in-memory SQLite database.
# demo
