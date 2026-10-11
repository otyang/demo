# API response

`github.com/otyang/demo/pkg/response` provides consistent JSON envelopes, pagination metadata,
typed application errors, HTTP response writers, and request binding with
[`uradical.io/go/binder`](https://pkg.go.dev/uradical.io/go/binder).

## Write responses

`WriteJSON` marshals non-nil bodies as JSON. When passed a nil body, it writes
the requested status code with an empty response body and does not set a
content type. This is useful for successful operations such as deletion:

```go
if err := response.WriteJSON(w, http.StatusOK, nil); err != nil {
	return err
}
```

## Bind requests

`Bind` binds a request into a struct with binder (`BindWithOptions`, at most
`MaxBodyBytes` = 1 MiB, unknown top-level keys rejected) and turns its
failures into response errors, with the statuses binder suggests:

```go
var input CreateUserRequest
if err := response.Bind(r, &input); err != nil {
	return err
}
```

| Request | Response |
|---|---|
| A body whose Content-Type is not JSON (`application/json` or `*+json`), or that has none | 415 `UNSUPPORTED_MEDIA_TYPE` |
| Fields that failed: an unknown or wrongly cased key, a value of the wrong type, or a missing `required` value | 400 `Invalid request`; `details` lists each as a `validation.FieldError` (`tag` is `unknown`, `type` or `required`) |
| Over `MaxBodyBytes` | 413 `PAYLOAD_TOO_LARGE` |
| Malformed or truncated JSON, an array, or a second value | 400 `Malformed request body` |
| An invalid target (a handler bug) | 500 |
| A body that could not be read, such as on a client disconnect | 400 `Request body could not be read` |

`details` has the same shape as a 422 validation failure:

```json
{"status":false,"error":{"code":"BAD_REQUEST","message":"Invalid request","details":[{"field":"count","tag":"type","message":"count has an invalid value"},{"field":"extra","tag":"unknown","message":"extra is not a known field"}]}}
```

binder's own messages name Go fields, so they stay in the logged cause and are
never sent. Tag client fields with binder's source tags (`body:"name"` for the
JSON body, `query` or `path` where the value comes from there); a field
without one, such as an ID taken from the access token, is never bound. A
request without a body (a GET bound from its query) needs no Content-Type.

Behaviour to be aware of:

- Keys match tags exactly: `{"Email":...}` does not bind `body:"email"`.
- Only top-level keys are checked; unknown keys inside a nested object are
  ignored.
- binder converts scalars: numbers and booleans bind into string fields
  (`{"name":12}` binds `"12"`), and numeric strings into number fields.
  Formats are checked by validation, not by the JSON type.
- Unknown **query** keys are not rejected; only body keys are. For a list
  filter, where a misspelt key would otherwise be ignored and widen the
  results, use `BindQueryStrict(r, &filter, "limit", "after")`. It answers
  400 `Invalid request` for each query key that no `query` tag (including
  those of embedded structs) or `extra` name covers, with `tag` `unknown`,
  and then binds as `Bind` does.

## Success response

`WriteSuccess` returns HTTP JSON using a top-level `status: true` and `data`:

```go
body := struct {
	Message string `json:"message"`
}{Message: "Hello, Ada!"}

if err := response.WriteSuccess(w, http.StatusOK, body); err != nil {
	log.Printf("write response: %v", err)
}
```

Response body:

```json
{"status":true,"data":{"message":"Hello, Ada!"}}
```

## Cursor-paginated list response

`WriteListCursor` writes list data with the `pagination.Meta` returned by the
cursor pagination helpers:

```go
if err := response.WriteListCursor(w, http.StatusOK, page.Data, page.Meta); err != nil {
	log.Printf("write list response: %v", err)
}
```

Example:

```json
{"status":true,"data":[{"id":"u1"}],"meta":{"limit":10,"hasMore":true,"nextCursor":"dXNlci0xMjM="}}
```

## Typed errors

Return typed errors from application or service code, preserving the original
cause where useful. In an HTTP handler, use `WriteErrorFrom` to map the error
to an HTTP status and safe JSON response:

```go
if err := service.GetUser(id); err != nil {
	if writeErr := response.WriteErrorFrom(w, err); writeErr != nil {
		log.Printf("write error response: %v", writeErr)
	}
	return
}
```

Available constructors:

| Constructor | HTTP status | Code |
|---|---:|---|
| `NewBadRequestError(message, details, cause)` | 400 | `BAD_REQUEST` |
| `NewValidationError(details, cause)` | 422 | `VALIDATION_ERROR` |
| `NewNotFoundError(message, cause)` | 404 | `NOT_FOUND` |
| `NewConflictError(message, details, cause)` | 409 | `CONFLICT` |
| `NewDatabaseError(cause)` | 500 | `DATABASE_ERROR` |
| `NewInternalServerError(cause)` | 500 | `INTERNAL_ERROR` |

For other HTTP error cases, use `NewError(status, code, message, details,
cause)` rather than adding a dedicated error type for every status.
`httpx.MethodNotAllowed` uses it for `METHOD_NOT_ALLOWED`, for example.

Failure responses have `status: false` and an `error` object:

```json
{"status":false,"error":{"code":"VALIDATION_ERROR","message":"Request validation failed","details":{"email":"invalid"}}}
```

The `details` argument on `WriteError` is optional. Internal
database causes and unknown errors are not exposed to clients; unknown errors
are returned as generic internal errors. Typed errors wrap their cause, so
callers can inspect them using `errors.Is` and `errors.As`.

For an untyped error response, call `WriteError` directly:

```go
if err := response.WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "Request is invalid"); err != nil {
	log.Printf("write error response: %v", err)
}
```
