package task_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otyang/demo/internal/bootstrap"
	"github.com/otyang/demo/internal/middleware"
	"github.com/otyang/demo/internal/task"
	"github.com/otyang/demo/pkg/store"
)

// api serves the task routes from a fresh in-memory SQLite database. Each
// request is authenticated as a member of org with permissions.
type api struct {
	t      *testing.T
	router http.Handler
}

func newAPI(t *testing.T, org string, permissions ...string) *api {
	t.Helper()
	db, err := store.New(store.Options{
		Type: store.SQLite, DSN: "file:" + t.Name() + "?mode=memory&cache=shared", MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return newAPIOn(t, db, org, permissions...)
}

func newAPIOn(t *testing.T, db *store.Store, org string, permissions ...string) *api {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	require.NoError(t, task.Migrate(context.Background(), db.BunDB, logger))

	repo, err := task.NewBunRepository(db.BunDB)
	require.NoError(t, err)
	service, err := task.NewService(repo)
	require.NoError(t, err)
	claims := middleware.JWTClaims{OrganizationID: org, Permissions: permissions}
	handler, err := task.NewHandler(service, middleware.TestClaims(claims))
	require.NoError(t, err)

	router := bootstrap.NewRouter(logger, nil)
	router.Mount("/api/v1/tasks", handler)
	return &api{t: t, router: router}
}

type envelope struct {
	Status bool            `json:"status"`
	Data   json.RawMessage `json:"data"`
	Meta   struct {
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

func (a *api) do(method, path, body string) (int, envelope) {
	a.t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	a.router.ServeHTTP(recorder, request)
	var result envelope
	if recorder.Body.Len() > 0 {
		require.NoError(a.t, json.Unmarshal(recorder.Body.Bytes(), &result), recorder.Body.String())
	}
	return recorder.Code, result
}

func (a *api) create(title string) task.Task {
	a.t.Helper()
	status, body := a.do(http.MethodPost, "/api/v1/tasks", `{"title":"`+title+`"}`)
	require.Equal(a.t, http.StatusCreated, status)
	var created task.Task
	require.NoError(a.t, json.Unmarshal(body.Data, &created))
	return created
}

func TestCreateGetUpdateDelete(t *testing.T) {
	a := newAPI(t, "org_a", task.PermissionRead, task.PermissionWrite)

	created := a.create("  Write the README  ")
	assert.True(t, strings.HasPrefix(created.ID, "task_"))
	assert.Equal(t, "Write the README", created.Title, "the title is trimmed")
	assert.False(t, created.Done)

	status, body := a.do(http.MethodGet, "/api/v1/tasks/"+created.ID, "")
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body.Data), `"title":"Write the README"`)
	assert.NotContains(t, string(body.Data), "org_a", "the organisation is not exposed")

	status, body = a.do(http.MethodPatch, "/api/v1/tasks/"+created.ID, `{"done":true}`)
	require.Equal(t, http.StatusOK, status)
	var updated task.Task
	require.NoError(t, json.Unmarshal(body.Data, &updated))
	assert.True(t, updated.Done)
	assert.Equal(t, "Write the README", updated.Title, "fields not sent are kept")

	status, _ = a.do(http.MethodDelete, "/api/v1/tasks/"+created.ID, "")
	assert.Equal(t, http.StatusNoContent, status)
	status, body = a.do(http.MethodGet, "/api/v1/tasks/"+created.ID, "")
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, "NOT_FOUND", body.Error.Code)
}

func TestListPaginatesNewestFirst(t *testing.T) {
	a := newAPI(t, "org_a", task.PermissionRead, task.PermissionWrite)
	for _, title := range []string{"one", "two", "three", "four", "five"} {
		a.create(title) // several share a created_at second; the ID breaks ties
	}

	var titles []string
	path := "/api/v1/tasks?limit=2"
	for pages := 0; ; pages++ {
		require.Less(t, pages, 5, "pagination did not end")
		status, body := a.do(http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, status)
		var page []task.Task
		require.NoError(t, json.Unmarshal(body.Data, &page))
		for _, item := range page {
			titles = append(titles, item.Title)
		}
		if !body.Meta.HasMore {
			break
		}
		path = "/api/v1/tasks?limit=2&after=" + body.Meta.NextCursor
	}
	assert.ElementsMatch(t, []string{"one", "two", "three", "four", "five"}, titles)
	assert.Len(t, titles, 5, "no task is listed twice")
}

func TestListFiltersByDone(t *testing.T) {
	a := newAPI(t, "org_a", task.PermissionRead, task.PermissionWrite)
	finished := a.create("finished")
	a.create("open")
	status, _ := a.do(http.MethodPatch, "/api/v1/tasks/"+finished.ID, `{"done":true}`)
	require.Equal(t, http.StatusOK, status)

	status, body := a.do(http.MethodGet, "/api/v1/tasks?done=true", "")
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body.Data), "finished")
	assert.NotContains(t, string(body.Data), `"open"`)
}

func TestOrganisationsOnlySeeTheirOwnTasks(t *testing.T) {
	db, err := store.New(store.Options{Type: store.SQLite, DSN: "file:isolation?mode=memory&cache=shared", MaxOpenConns: 1, MaxIdleConns: 1})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	a := newAPIOn(t, db, "org_a", task.PermissionRead, task.PermissionWrite)
	b := newAPIOn(t, db, "org_b", task.PermissionRead, task.PermissionWrite)

	secret := a.create("org a only")

	status, _ := b.do(http.MethodGet, "/api/v1/tasks/"+secret.ID, "")
	assert.Equal(t, http.StatusNotFound, status)
	status, _ = b.do(http.MethodDelete, "/api/v1/tasks/"+secret.ID, "")
	assert.Equal(t, http.StatusNotFound, status)
	_, body := b.do(http.MethodGet, "/api/v1/tasks", "")
	assert.Equal(t, "[]", string(body.Data))
}

func TestRequestErrors(t *testing.T) {
	a := newAPI(t, "org_a", task.PermissionRead, task.PermissionWrite)
	for name, test := range map[string]struct {
		method, path, body string
		status             int
		code, field        string
	}{
		"missing title":  {http.MethodPost, "/api/v1/tasks", `{"title":"   "}`, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "title"},
		"title too long": {http.MethodPost, "/api/v1/tasks", `{"title":"` + strings.Repeat("x", 201) + `"}`, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "title"},
		"unknown field":  {http.MethodPost, "/api/v1/tasks", `{"title":"a","colour":"red"}`, http.StatusBadRequest, "BAD_REQUEST", "colour"},
		"malformed body": {http.MethodPost, "/api/v1/tasks", `{`, http.StatusBadRequest, "BAD_REQUEST", ""},
		"unknown filter": {http.MethodGet, "/api/v1/tasks?dnoe=true", "", http.StatusBadRequest, "BAD_REQUEST", "dnoe"},
		"bad limit":      {http.MethodGet, "/api/v1/tasks?limit=0", "", http.StatusBadRequest, "BAD_REQUEST", ""},
		"bad cursor":     {http.MethodGet, "/api/v1/tasks?after=!!", "", http.StatusBadRequest, "BAD_REQUEST", ""},
		"unknown task":   {http.MethodPatch, "/api/v1/tasks/task_nope", `{"done":true}`, http.StatusNotFound, "NOT_FOUND", ""},
		"wrong method":   {http.MethodPut, "/api/v1/tasks/task_nope", `{}`, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", ""},
	} {
		t.Run(name, func(t *testing.T) {
			status, body := a.do(test.method, test.path, test.body)
			assert.Equal(t, test.status, status)
			assert.Equal(t, test.code, body.Error.Code)
			if test.field != "" {
				require.NotEmpty(t, body.Error.Details)
				assert.Equal(t, test.field, body.Error.Details[0].Field)
			}
		})
	}
}

func TestPermissionsAndOrganisationAreRequired(t *testing.T) {
	readOnly := newAPI(t, "org_a", task.PermissionRead)
	status, body := readOnly.do(http.MethodPost, "/api/v1/tasks", `{"title":"a"}`)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "FORBIDDEN", body.Error.Code)

	noOrganisation := newAPI(t, "", task.PermissionRead)
	status, body = noOrganisation.do(http.MethodGet, "/api/v1/tasks", "")
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "ORG_SETUP_REQUIRED", body.Error.Code)
}

func TestTimestampsAreWholeSecondsInUTC(t *testing.T) {
	a := newAPI(t, "org_a", task.PermissionRead, task.PermissionWrite)
	created := a.create("a")
	assert.Equal(t, time.UTC, created.CreatedAt.Location())
	assert.Zero(t, created.CreatedAt.Nanosecond())
}
