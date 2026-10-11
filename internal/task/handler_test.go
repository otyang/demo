package task

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otyang/demo/internal/middleware"
	"github.com/otyang/demo/pkg/pagination"
)

// handlerFixture serves the task routes over a fakeRepository, mounted
// alone so each test sees exactly what the handler does.
type handlerFixture struct {
	t      *testing.T
	repo   *fakeRepository
	router http.Handler
}

func newHandlerFixture(t *testing.T, claims middleware.JWTClaims, tasks ...Task) *handlerFixture {
	t.Helper()
	repo := newFakeRepository(tasks...)
	handler, err := NewHandler(newTestService(t, repo), middleware.TestClaims(claims))
	require.NoError(t, err)
	return &handlerFixture{t: t, repo: repo, router: handler}
}

// member is a caller in org_a with permissions.
func member(permissions ...string) middleware.JWTClaims {
	return middleware.JWTClaims{OrganizationID: "org_a", Permissions: permissions}
}

type handlerResponse struct {
	Status bool            `json:"status"`
	Data   json.RawMessage `json:"data"`
	Meta   *struct {
		Limit      int    `json:"limit"`
		HasMore    bool   `json:"hasMore"`
		NextCursor string `json:"nextCursor"`
	} `json:"meta"`
	Error struct {
		Code    string `json:"code"`
		Details []struct {
			Field string `json:"field"`
		} `json:"details"`
	} `json:"error"`
}

// serve sends a request; a non-empty body is sent as JSON.
func (f *handlerFixture) serve(method, path, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func (f *handlerFixture) do(method, path, body string) (int, handlerResponse) {
	f.t.Helper()
	recorder := f.serve(method, path, body)
	var result handlerResponse
	if recorder.Body.Len() > 0 {
		require.NoError(f.t, json.Unmarshal(recorder.Body.Bytes(), &result), recorder.Body.String())
	}
	return recorder.Code, result
}

func decodeData[T any](t *testing.T, body handlerResponse) T {
	t.Helper()
	var value T
	require.NoError(t, json.Unmarshal(body.Data, &value), string(body.Data))
	return value
}

func TestNewHandlerRequiresDependencies(t *testing.T) {
	service := newTestService(t, newFakeRepository())
	authenticate := middleware.TestClaims(member())

	_, err := NewHandler(nil, authenticate)
	assert.Error(t, err)
	_, err = NewHandler(service, nil)
	assert.Error(t, err)
}

func TestHandlerRoutePermissions(t *testing.T) {
	existing := Task{ID: "task_1", OrganisationID: "org_a", Title: "a"}
	routes := []struct {
		method, path, body, permission string
		success                        int
	}{
		{http.MethodGet, "/", "", PermissionRead, http.StatusOK},
		{http.MethodGet, "/task_1", "", PermissionRead, http.StatusOK},
		{http.MethodPost, "/", `{"title":"a"}`, PermissionWrite, http.StatusCreated},
		{http.MethodPatch, "/task_1", `{"done":true}`, PermissionWrite, http.StatusOK},
		{http.MethodDelete, "/task_1", "", PermissionWrite, http.StatusNoContent},
	}
	for _, route := range routes {
		name := route.method + " " + route.path
		t.Run(name+" with permission", func(t *testing.T) {
			f := newHandlerFixture(t, member(route.permission), existing)
			assert.Equal(t, route.success, f.serve(route.method, route.path, route.body).Code)
		})
		t.Run(name+" without permission", func(t *testing.T) {
			other := PermissionRead
			if route.permission == PermissionRead {
				other = PermissionWrite
			}
			f := newHandlerFixture(t, member(other), existing)
			status, body := f.do(route.method, route.path, route.body)
			assert.Equal(t, http.StatusForbidden, status)
			assert.Equal(t, "FORBIDDEN", body.Error.Code)
			assert.Empty(t, f.repo.calls, "a forbidden request never reaches the service")
		})
		t.Run(name+" without organisation", func(t *testing.T) {
			f := newHandlerFixture(t, middleware.JWTClaims{OrganizationID: "  ", Permissions: []string{PermissionRead, PermissionWrite}}, existing)
			status, body := f.do(route.method, route.path, route.body)
			assert.Equal(t, http.StatusForbidden, status)
			assert.Equal(t, "ORG_SETUP_REQUIRED", body.Error.Code)
			assert.Empty(t, f.repo.calls)
		})
	}
}

func TestHandlerRequiresClaims(t *testing.T) {
	passThrough := func(next http.Handler) http.Handler { return next }
	handler, err := NewHandler(newTestService(t, newFakeRepository()), passThrough)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestHandlerCreate(t *testing.T) {
	f := newHandlerFixture(t, member(PermissionWrite))

	recorder := f.serve(http.MethodPost, "/", `{"title":" Plan ","notes":"Q3"}`)
	require.Equal(t, http.StatusCreated, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

	var body handlerResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.True(t, body.Status)
	assert.Nil(t, body.Meta)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(body.Data, &fields))
	assert.ElementsMatch(t, []string{"id", "title", "notes", "done", "createdAt", "updatedAt"}, keys(fields),
		"the organisation is never in a response")
	assert.Equal(t, "Plan", fields["title"])
	assert.Equal(t, "Q3", fields["notes"])
	assert.Equal(t, false, fields["done"])
	assert.Equal(t, "2026-03-04T09:30:15Z", fields["createdAt"])

	id, _ := fields["id"].(string)
	require.Contains(t, f.repo.tasks, id)
	assert.Equal(t, "org_a", f.repo.tasks[id].OrganisationID, "the task belongs to the caller's organisation")
}

func TestHandlerCreateBodyErrors(t *testing.T) {
	for name, test := range map[string]struct {
		body, contentType string
		status            int
		code, field       string
	}{
		"not JSON":    {`{"title":"a"}`, "text/plain", http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", ""},
		"malformed":   {`{"title":`, "application/json", http.StatusBadRequest, "BAD_REQUEST", ""},
		"unknown key": {`{"title":"a","owner":"me"}`, "application/json", http.StatusBadRequest, "BAD_REQUEST", "owner"},
		"wrong type":  {`{"title":["x"]}`, "application/json", http.StatusBadRequest, "BAD_REQUEST", "title"},
		"invalid":     {`{"title":""}`, "application/json", http.StatusUnprocessableEntity, "VALIDATION_ERROR", "title"},
		"too large":   {`{"title":"` + strings.Repeat("x", 2<<20) + `"}`, "application/json", http.StatusRequestEntityTooLarge, "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			f := newHandlerFixture(t, member(PermissionWrite))
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			recorder := httptest.NewRecorder()
			f.router.ServeHTTP(recorder, request)

			assert.Equal(t, test.status, recorder.Code)
			var body handlerResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body), recorder.Body.String())
			assert.False(t, body.Status)
			if test.code != "" {
				assert.Equal(t, test.code, body.Error.Code)
			}
			if test.field != "" {
				require.NotEmpty(t, body.Error.Details)
				assert.Equal(t, test.field, body.Error.Details[0].Field)
			}
			assert.Empty(t, f.repo.tasks, "nothing is stored")
		})
	}
}

func TestHandlerGet(t *testing.T) {
	stored := Task{ID: "task_1", OrganisationID: "org_a", Title: "a", Notes: "n", Done: true}
	f := newHandlerFixture(t, member(PermissionRead), stored, Task{ID: "task_2", OrganisationID: "org_b"})

	status, body := f.do(http.MethodGet, "/task_1", "")
	require.Equal(t, http.StatusOK, status)
	got := decodeData[Task](t, body)
	assert.Equal(t, "task_1", got.ID, "the ID comes from the path")
	assert.Equal(t, "a", got.Title)
	assert.True(t, got.Done)

	status, body = f.do(http.MethodGet, "/task_2", "")
	assert.Equal(t, http.StatusNotFound, status, "another organisation's task is not found")
	assert.Equal(t, "NOT_FOUND", body.Error.Code)
}

func TestHandlerList(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := newHandlerFixture(t, member(PermissionRead))
	f.repo.listResult = []Task{
		{ID: "task_3", Title: "c", CreatedAt: base.Add(3 * time.Second)},
		{ID: "task_2", Title: "b", CreatedAt: base.Add(2 * time.Second)},
		{ID: "task_1", Title: "a", CreatedAt: base.Add(1 * time.Second)},
	}

	status, body := f.do(http.MethodGet, "/?limit=2", "")
	require.Equal(t, http.StatusOK, status)
	page := decodeData[[]Task](t, body)
	assert.Equal(t, []string{"task_3", "task_2"}, taskIDs(page))
	require.NotNil(t, body.Meta)
	assert.Equal(t, 2, body.Meta.Limit)
	assert.True(t, body.Meta.HasMore)
	assert.Equal(t, pagination.EncodeCursor(pagination.FormatTimeIDCursorValue(base.Add(2*time.Second), "task_2")), body.Meta.NextCursor)

	assert.Equal(t, "org_a", f.repo.lastQuery.OrganisationID)
	assert.Equal(t, 3, f.repo.lastQuery.Limit)
	assert.Nil(t, f.repo.lastQuery.Done)
	assert.Nil(t, f.repo.lastQuery.After)
}

func TestHandlerListPassesQuery(t *testing.T) {
	createdAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	after := pagination.EncodeCursor(pagination.FormatTimeIDCursorValue(createdAt, "task_7"))

	for name, test := range map[string]struct {
		query string
		done  *bool
	}{
		"done":      {"done=true", ptr(true)},
		"open":      {"done=false", ptr(false)},
		"no filter": {"", nil},
	} {
		t.Run(name, func(t *testing.T) {
			f := newHandlerFixture(t, member(PermissionRead))
			status, _ := f.do(http.MethodGet, "/?after="+after+"&"+test.query, "")
			require.Equal(t, http.StatusOK, status)
			assert.Equal(t, test.done, f.repo.lastQuery.Done)
			require.NotNil(t, f.repo.lastQuery.After)
			assert.Equal(t, "task_7", f.repo.lastQuery.After.ID)
			assert.True(t, createdAt.Equal(f.repo.lastQuery.After.CreatedAt))
		})
	}
}

func TestHandlerListEmpty(t *testing.T) {
	f := newHandlerFixture(t, member(PermissionRead))
	status, body := f.do(http.MethodGet, "/", "")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "[]", string(body.Data))
	require.NotNil(t, body.Meta)
	assert.Equal(t, defaultLimit, body.Meta.Limit)
	assert.False(t, body.Meta.HasMore)
	assert.Empty(t, body.Meta.NextCursor)
}

func TestHandlerListQueryErrors(t *testing.T) {
	for name, test := range map[string]struct {
		query, field string
	}{
		"unknown key":        {"status=open", "status"},
		"done not a bool":    {"done=maybe", "done"},
		"limit not a number": {"limit=ten", ""},
		"limit zero":         {"limit=0", ""},
		"limit negative":     {"limit=-5", ""},
		"bad cursor":         {"after=!!", ""},
	} {
		t.Run(name, func(t *testing.T) {
			f := newHandlerFixture(t, member(PermissionRead))
			status, body := f.do(http.MethodGet, "/?"+test.query, "")
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Equal(t, "BAD_REQUEST", body.Error.Code)
			if test.field != "" {
				require.NotEmpty(t, body.Error.Details)
				assert.Equal(t, test.field, body.Error.Details[0].Field)
			}
			assert.Empty(t, f.repo.calls)
		})
	}
}

func TestHandlerUpdate(t *testing.T) {
	stored := Task{ID: "task_1", OrganisationID: "org_a", Title: "old", Notes: "keep"}
	f := newHandlerFixture(t, member(PermissionWrite), stored)

	status, body := f.do(http.MethodPatch, "/task_1", `{"title":"new","done":true}`)
	require.Equal(t, http.StatusOK, status)
	updated := decodeData[Task](t, body)
	assert.Equal(t, "new", updated.Title)
	assert.Equal(t, "keep", updated.Notes, "keys not sent are kept")
	assert.True(t, updated.Done)
	assert.Equal(t, "new", f.repo.tasks["task_1"].Title)
}

func TestHandlerUpdateErrors(t *testing.T) {
	for name, test := range map[string]struct {
		path, body string
		status     int
		code       string
	}{
		"unknown task": {"/task_missing", `{"done":true}`, http.StatusNotFound, "NOT_FOUND"},
		"foreign task": {"/task_2", `{"done":true}`, http.StatusNotFound, "NOT_FOUND"},
		"blank title":  {"/task_1", `{"title":"  "}`, http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		"unknown key":  {"/task_1", `{"priority":1}`, http.StatusBadRequest, "BAD_REQUEST"},
		"malformed":    {"/task_1", `[`, http.StatusBadRequest, "BAD_REQUEST"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newHandlerFixture(t, member(PermissionWrite),
				Task{ID: "task_1", OrganisationID: "org_a", Title: "a"},
				Task{ID: "task_2", OrganisationID: "org_b", Title: "b"})
			status, body := f.do(http.MethodPatch, test.path, test.body)
			assert.Equal(t, test.status, status)
			assert.Equal(t, test.code, body.Error.Code)
			assert.NotContains(t, f.repo.calls, "Update")
		})
	}
}

func TestHandlerDelete(t *testing.T) {
	f := newHandlerFixture(t, member(PermissionWrite),
		Task{ID: "task_1", OrganisationID: "org_a"},
		Task{ID: "task_2", OrganisationID: "org_b"})

	recorder := f.serve(http.MethodDelete, "/task_1", "")
	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Zero(t, recorder.Body.Len(), "204 has no body")
	assert.NotContains(t, f.repo.tasks, "task_1")

	status, body := f.do(http.MethodDelete, "/task_1", "")
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, "NOT_FOUND", body.Error.Code)

	status, _ = f.do(http.MethodDelete, "/task_2", "")
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, f.repo.tasks, "task_2", "another organisation's task is kept")
}

func TestHandlerDatabaseErrorsHideTheCause(t *testing.T) {
	f := newHandlerFixture(t, member(PermissionRead, PermissionWrite))
	f.repo.listErr = errors.New("pq: password authentication failed for user admin")

	recorder := f.serve(http.MethodGet, "/", "")
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "DATABASE_ERROR")
	assert.NotContains(t, recorder.Body.String(), "password", "internal details are not sent to the client")
}

func TestHandlerUsesTheCallersOrganisation(t *testing.T) {
	f := newHandlerFixture(t, middleware.JWTClaims{OrganizationID: "org_z", Permissions: []string{PermissionRead}})
	status, _ := f.do(http.MethodGet, "/", "")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "org_z", f.repo.lastQuery.OrganisationID)
}

func keys(m map[string]any) []string {
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, key)
	}
	return result
}
