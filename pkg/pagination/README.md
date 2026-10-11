# Cursor pagination

`github.com/otyang/demo/pkg/pagination` provides validated forward-cursor parameters, opaque
URL-safe cursors, and generic JSON page metadata. It expects queries to fetch
one more row than the requested limit to determine whether another page
exists.

## Normalize query parameters

Use `Normalize` at the API boundary to apply a default limit, cap excessive
limits, and reject invalid cursors:

```go
import (
	"github.com/otyang/demo/pkg/pagination"
	"github.com/otyang/demo/pkg/response"
)

params, err := pagination.Normalize(pagination.Params{
	Limit: queryLimit,
	After: queryAfter,
}, 25, 100)
if err != nil {
	return response.NewBadRequestError("Invalid pagination parameters", nil, err)
}
decodedAfter := ""
if params.After != "" {
	decodedAfter, err = pagination.DecodeCursor(params.After)
	if err != nil {
		return response.NewBadRequestError("Invalid pagination cursor", nil, err)
	}
}
```

Pass `decodedAfter` into a stable database keyset query. For example, order by
`(created_at, id)` and use the same deterministic ordering in the cursor query;
offset pagination should not be mixed with cursors.

For a cursor based on a timestamp and a string key, use
`FormatTimeIDCursorValue` to create the payload and
`ParseTimeIDCursorValue` to parse a decoded payload. Encode the formatted
payload with `EncodeCursor` before returning it to a client:

```go
value := pagination.FormatTimeIDCursorValue(item.CreatedAt, item.ID)
opaqueCursor := pagination.EncodeCursor(value)

decoded, err := pagination.DecodeCursor(opaqueCursor)
if err != nil {
	return err
}
createdAt, id, err := pagination.ParseTimeIDCursorValue(decoded)
if err != nil {
	return err
}
```

## Build a response page

Fetch `params.Limit + 1` records. `Build` trims the extra record, determines
`hasMore`, and encodes the last returned row's cursor as `nextCursor`.
Provide total count only if the query already has it or the extra count query
is acceptable:

```go
import (
	"context"
	"net/http"

	"github.com/otyang/demo/pkg/pagination"
	"github.com/otyang/demo/pkg/response"
)

type User struct {
	ID string `json:"id"`
}

// ctx, repository, params, and decodedAfter are prepared by the handler.
rows, err := repository.ListAfter(ctx, decodedAfter, params.Limit+1)
if err != nil {
	return err
}

page, err := pagination.Build(rows, params, func(item User) string {
	return item.ID
}, nil)
if err != nil {
	return err
}
return response.WriteListCursor(w, http.StatusOK, page.Data, page.Meta)
```

Example response:

```json
{
  "status": true,
  "data": [{"id": "user-123"}],
  "meta": {
    "limit": 25,
    "hasMore": true,
    "nextCursor": "dXNlci0xMjM="
  }
}
```

`EncodeCursor` and `DecodeCursor` use URL-safe base64 encoding. This makes
cursor values convenient to transport, but it does not encrypt or authenticate
them. Treat cursors as untrusted input and validate decoded values in the
repository query.
