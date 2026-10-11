# task

The example service: a to-do list per organisation. Copy it to start a new
service.

## Routes

| Method | Path | Permission | Does |
| --- | --- | --- | --- |
| `GET` | `/api/v1/tasks` | `tasks:read` | List tasks, newest first. Query: `done`, `limit`, `after` |
| `POST` | `/api/v1/tasks` | `tasks:write` | Create a task: `{"title": "...", "notes": "..."}` |
| `GET` | `/api/v1/tasks/{id}` | `tasks:read` | Get a task |
| `PATCH` | `/api/v1/tasks/{id}` | `tasks:write` | Change any of `title`, `notes`, `done` |
| `DELETE` | `/api/v1/tasks/{id}` | `tasks:write` | Delete a task |

Every route also needs an organisation (`org_id`) in the caller's token, and
only ever sees that organisation's tasks. The routes are documented in
[doc/openapi.yaml](../../doc/openapi.yaml); keep it in step when you change
them (`internal/api`'s tests check that it lists exactly these routes).

## Layers

| File | Layer | Uses |
| --- | --- | --- |
| [model.go](model.go) | `Task`, and the request inputs with their `body`, `query` and `validate` tags | |
| [repository.go](repository.go) | `Repository` interface and its Bun implementation; `Migrate` | `pkg/store` |
| [service.go](service.go) | Validation, IDs, pagination, error mapping | `pkg/validation`, `pkg/ids`, `pkg/pagination`, `pkg/response` |
| [handler.go](handler.go) | Routes, permissions, binding and writing | `internal/middleware`, `pkg/httpx`, `pkg/response` |

Each layer only knows the one below it:

- The **handler** declares the routes on a chi router, behind
  authentication, `middleware.RequireOrganization` and
  `middleware.RequirePermission`. It binds the request (`response.Bind`,
  `response.BindQueryStrict`, `httpx.PageParams`), takes the organisation
  from the JWT claims, calls the service and writes the result
  (`response.WriteSuccess`, `response.WriteListCursor`). It returns errors
  rather than writing them.
- The **service** trims and validates input (422 with each failed field),
  generates `task_` IDs, builds cursor pages, and turns repository errors
  into response errors: `ErrNotFound` becomes 404, anything else a 500
  `DATABASE_ERROR`.
- The **repository** runs the SQL. Every query filters by organisation.

Tests use the real repository on in-memory SQLite, so there are no mocks:
see [task_test.go](task_test.go).

## Pagination

Lists are ordered by `created_at DESC, id DESC`, and the cursor is the last
task's creation time and ID. Times are stored in UTC and whole seconds, so
they compare the same way on SQLite (where they are text) and PostgreSQL;
the ID breaks ties within a second.

## Schema

[migrations](migrations) holds the SQL, applied at startup by `Migrate` and
recorded in `task_migrations`. Name new files
`<number>_<description>.up.sql` and `.down.sql`, and keep the SQL valid on
both SQLite and PostgreSQL.
