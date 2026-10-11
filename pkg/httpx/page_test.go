package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/otyang/demo/pkg/pagination"
	"github.com/otyang/demo/pkg/response"
)

func TestPageParams(t *testing.T) {
	page, err := PageParams(httptest.NewRequest(http.MethodGet, "/?limit=10&after=cursor_1", nil))
	if err != nil || page != (pagination.Params{Limit: 10, After: "cursor_1"}) {
		t.Fatalf("PageParams = %+v, %v", page, err)
	}
	page, err = PageParams(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil || page != (pagination.Params{}) {
		t.Fatalf("PageParams without parameters = %+v, %v; want the zero value", page, err)
	}
	for _, query := range []string{"limit=0", "limit=-1", "limit=ten"} {
		_, err := PageParams(httptest.NewRequest(http.MethodGet, "/?"+query, nil))
		var responseErr response.ResponseError
		if !errors.As(err, &responseErr) || responseErr.HTTPStatus() != http.StatusBadRequest {
			t.Errorf("%s: err = %v, want 400", query, err)
		}
	}
}
