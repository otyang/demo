# doc

The API reference: [openapi.yaml](openapi.yaml), an OpenAPI 3.1 file
describing every route the API serves, and a small command that serves it
rendered with [Scalar](https://scalar.com).

```sh
go run ./doc                      # http://localhost:8080/docs
go run ./doc -addr 127.0.0.1:9000 # or anywhere else
```

| Path | Serves |
| --- | --- |
| `GET /docs` | The reference page |
| `GET /docs/openapi.yaml` | The spec, for clients and code generators |

The command is self-contained: the spec is embedded at build time and it
uses only the standard library. The page loads Scalar from jsDelivr, pinned
to a version; update it on purpose.

## Trying requests from the page

The spec's server is `http://localhost:3000`, so run the API too
(`go run ./cmd/api`). The page calls it from `localhost:8080`, which
`.example.env` allows in `DEMO_CORS_ALLOWED_ORIGINS`.

## Keeping it in step with the code

When you add, remove or rename a route, update `openapi.yaml`.
[internal/api](../internal/api/api_test.go)'s tests walk the router and
fail when the routes served and the operations documented differ, so
`go test ./...` catches a forgotten update.
