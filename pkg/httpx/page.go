package httpx

import (
	"net/http"
	"strconv"

	"github.com/otyang/demo/pkg/pagination"
	"github.com/otyang/demo/pkg/response"
)

// PageParams reads the list pagination query parameters, limit and after.
// A limit that is not a positive integer answers 400. The service applies the
// default and maximum limit and decodes the cursor.
func PageParams(r *http.Request) (pagination.Params, error) {
	query := r.URL.Query()
	page := pagination.Params{After: query.Get("after")}
	if rawLimit := query.Get("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 1 {
			return pagination.Params{}, response.NewBadRequestError("limit must be a positive integer", nil, err)
		}
		page.Limit = limit
	}
	return page, nil
}
