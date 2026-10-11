package response

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/otyang/demo/pkg/validation"
)

type bindTarget struct {
	Name     string `body:"name"`
	Count    int    `body:"count"`
	Internal string
}

func newBindRequest(contentType, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	return request
}

func requireBindError(t *testing.T, err error, status int, message string) ResponseError {
	t.Helper()
	responseErr, ok := errors.AsType[ResponseError](err)
	if !ok {
		t.Fatalf("Bind() error = %v, want a response error", err)
	}
	if responseErr.HTTPStatus() != status || responseErr.ErrorPayload().Message != message {
		t.Fatalf("Bind() = %d %q, want %d %q", responseErr.HTTPStatus(), responseErr.ErrorPayload().Message, status, message)
	}
	return responseErr
}

func TestBindBindsJSON(t *testing.T) {
	for _, contentType := range []string{"application/json", "application/json; charset=utf-8", "application/merge-patch+json"} {
		var target bindTarget
		if err := Bind(newBindRequest(contentType, `{"name":"Ada","count":3}`), &target); err != nil {
			t.Fatalf("Bind(%s) error = %v", contentType, err)
		}
		if target.Name != "Ada" || target.Count != 3 {
			t.Errorf("Bind(%s) target = %+v", contentType, target)
		}
	}
}

func TestBindNeedsNoContentTypeWithoutABody(t *testing.T) {
	var target bindTarget
	if err := Bind(httptest.NewRequest(http.MethodGet, "/", nil), &target); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
}

func TestBindNeverSetsUntaggedFields(t *testing.T) {
	request := newBindRequest("application/json", `{"name":"Ada"}`)
	request.URL.RawQuery = "Internal=x"
	request.Header.Set("Internal", "x")
	var target bindTarget
	if err := Bind(request, &target); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if target.Internal != "" {
		t.Error("a field without a binding tag must never be bound")
	}
}

func TestBindRejections(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		status      int
		message     string
	}{
		{"no Content-Type", "", `{"name":"Ada"}`, http.StatusUnsupportedMediaType, "Content-Type must be application/json"},
		{"form body", "application/x-www-form-urlencoded", `name=Ada`, http.StatusUnsupportedMediaType, "Content-Type must be application/json"},
		{"text body", "text/plain", `{"name":"Ada"}`, http.StatusUnsupportedMediaType, "Content-Type must be application/json"},
		{"truncated", "application/json", `{"name":`, http.StatusBadRequest, "Malformed request body"},
		{"second value", "application/json", `{"name":"Ada"} {}`, http.StatusBadRequest, "Malformed request body"},
		{"array", "application/json", `[1]`, http.StatusBadRequest, "Malformed request body"},
		{"unknown key", "application/json", `{"name":"Ada","Internal":"x"}`, http.StatusBadRequest, "Invalid request"},
		{"wrong case", "application/json", `{"Name":"Ada"}`, http.StatusBadRequest, "Invalid request"},
		{"invalid value", "application/json", `{"count":"many"}`, http.StatusBadRequest, "Invalid request"},
		{"too large", "application/json", `{"name":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, http.StatusRequestEntityTooLarge, "payload too large (limit 1048576 bytes)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var target bindTarget
			requireBindError(t, Bind(newBindRequest(test.contentType, test.body), &target), test.status, test.message)
		})
	}
}

func TestBindNamesFieldsThatFailed(t *testing.T) {
	var target bindTarget
	err := Bind(newBindRequest("application/json", `{"count":"many","extra":1}`), &target)
	responseErr := requireBindError(t, err, http.StatusBadRequest, "Invalid request")
	fields, ok := responseErr.ErrorPayload().Details.(validation.Errors)
	if !ok {
		t.Fatalf("details = %T, want validation.Errors", responseErr.ErrorPayload().Details)
	}
	want := validation.Errors{
		{Field: "count", Tag: "type", Message: "count has an invalid value"},
		{Field: "extra", Tag: "unknown", Message: "extra is not a known field"},
	}
	if len(fields) != len(want) {
		t.Fatalf("details = %+v, want %+v", fields, want)
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Errorf("details[%d] = %+v, want %+v", i, fields[i], want[i])
		}
	}
}

func TestBindReportsAnInvalidTargetAsInternal(t *testing.T) {
	err := Bind(newBindRequest("application/json", `{}`), bindTarget{})
	requireBindError(t, err, http.StatusInternalServerError, "An internal server error occurred")
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestBindReportsAnUnreadableBody(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", failingReader{})
	request.Header.Set("Content-Type", "application/json")
	request.ContentLength = -1
	var target bindTarget
	requireBindError(t, Bind(request, &target), http.StatusBadRequest, "Request body could not be read")
}

type queryFilterBase struct {
	Name string `query:"name"`
}

type queryFilter struct {
	queryFilterBase
	Org     string `query:"org,omitempty"`
	Skipped string `query:"-"`
}

func TestBindQueryStrictBindsKnownKeys(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/?name=Ada&org=org_1&limit=5", nil)
	var target queryFilter
	if err := BindQueryStrict(request, &target, "limit"); err != nil {
		t.Fatalf("BindQueryStrict() error = %v", err)
	}
	if target.Name != "Ada" || target.Org != "org_1" {
		t.Errorf("target = %+v, want the embedded and own query fields bound", target)
	}
}

func TestBindQueryStrictRejectsUnknownKeys(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/?organisation=org_1&name=Ada&Skipped=x&-=y", nil)
	var target queryFilter
	err := BindQueryStrict(request, &target)
	responseErr := requireBindError(t, err, http.StatusBadRequest, "Invalid request")
	fields, ok := responseErr.ErrorPayload().Details.(validation.Errors)
	if !ok {
		t.Fatalf("details = %T, want validation.Errors", responseErr.ErrorPayload().Details)
	}
	want := validation.Errors{
		{Field: "-", Tag: "unknown", Message: "- is not a known query parameter"},
		{Field: "Skipped", Tag: "unknown", Message: "Skipped is not a known query parameter"},
		{Field: "organisation", Tag: "unknown", Message: "organisation is not a known query parameter"},
	}
	if len(fields) != len(want) {
		t.Fatalf("details = %+v, want %+v", fields, want)
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Errorf("details[%d] = %+v, want %+v", i, fields[i], want[i])
		}
	}
	if target.Name != "" {
		t.Errorf("target = %+v, want nothing bound after a rejection", target)
	}
}
