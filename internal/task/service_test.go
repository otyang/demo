package task

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otyang/demo/pkg/pagination"
	"github.com/otyang/demo/pkg/response"
	"github.com/otyang/demo/pkg/validation"
)

// fakeRepository is an in-memory Repository that records its calls and can
// be told to fail.
type fakeRepository struct {
	tasks map[string]*Task

	createErr, getErr, listErr, updateErr, deleteErr error
	// listResult, when set, is what List returns instead of the stored tasks.
	listResult []Task

	calls     []string
	lastQuery ListQuery
}

func newFakeRepository(tasks ...Task) *fakeRepository {
	repo := &fakeRepository{tasks: map[string]*Task{}}
	for _, task := range tasks {
		repo.tasks[task.ID] = &task
	}
	return repo
}

func (r *fakeRepository) Create(_ context.Context, task *Task) error {
	r.calls = append(r.calls, "Create")
	if r.createErr != nil {
		return r.createErr
	}
	stored := *task
	r.tasks[task.ID] = &stored
	return nil
}

func (r *fakeRepository) Get(_ context.Context, organisationID, id string) (*Task, error) {
	r.calls = append(r.calls, "Get")
	if r.getErr != nil {
		return nil, r.getErr
	}
	task, ok := r.tasks[id]
	if !ok || task.OrganisationID != organisationID {
		return nil, ErrNotFound
	}
	found := *task
	return &found, nil
}

func (r *fakeRepository) List(_ context.Context, query ListQuery) ([]Task, error) {
	r.calls = append(r.calls, "List")
	r.lastQuery = query
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.listResult, nil
}

func (r *fakeRepository) Update(_ context.Context, task *Task) error {
	r.calls = append(r.calls, "Update")
	if r.updateErr != nil {
		return r.updateErr
	}
	stored := *task
	r.tasks[task.ID] = &stored
	return nil
}

func (r *fakeRepository) Delete(_ context.Context, organisationID, id string) error {
	r.calls = append(r.calls, "Delete")
	if r.deleteErr != nil {
		return r.deleteErr
	}
	task, ok := r.tasks[id]
	if !ok || task.OrganisationID != organisationID {
		return ErrNotFound
	}
	delete(r.tasks, id)
	return nil
}

// clock is the fixed time the service under test reads, in a non-UTC zone
// and with a fraction of a second, so tests can see it normalised.
var clock = time.Date(2026, 3, 4, 10, 30, 15, 987654321, time.FixedZone("WAT", 3600))

func newTestService(t *testing.T, repo Repository) *Service {
	t.Helper()
	service, err := NewService(repo)
	require.NoError(t, err)
	service.now = func() time.Time { return clock }
	return service
}

func ptr[T any](value T) *T { return &value }

// requireStatus asserts err is a response error with status.
func requireStatus(t *testing.T, err error, status int) {
	t.Helper()
	require.Error(t, err)
	responseErr, ok := errors.AsType[response.ResponseError](err)
	require.True(t, ok, "%v is not a response error", err)
	assert.Equal(t, status, responseErr.HTTPStatus())
}

// requireInvalidField asserts err is a validation error naming field.
func requireInvalidField(t *testing.T, err error, field string) {
	t.Helper()
	requireStatus(t, err, http.StatusUnprocessableEntity)
	validationErr, ok := errors.AsType[*response.ValidationError](err)
	require.True(t, ok)
	fields, ok := validationErr.ErrorPayload().Details.(validation.Errors)
	require.True(t, ok)
	require.NotEmpty(t, fields)
	assert.Equal(t, field, fields[0].Field)
}

func TestNewServiceRequiresRepository(t *testing.T) {
	_, err := NewService(nil)
	assert.Error(t, err)
}

func TestServiceCreate(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(t, repo)

	created, err := service.Create(context.Background(), "org_a", CreateInput{Title: "  Ship it  ", Notes: "  soon \n"})
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(created.ID, idPrefix))
	assert.Equal(t, "org_a", created.OrganisationID)
	assert.Equal(t, "Ship it", created.Title)
	assert.Equal(t, "soon", created.Notes)
	assert.False(t, created.Done)

	want := time.Date(2026, 3, 4, 9, 30, 15, 0, time.UTC)
	assert.Equal(t, want, created.CreatedAt, "created_at is UTC, truncated to the second")
	assert.Equal(t, want, created.UpdatedAt)

	require.Contains(t, repo.tasks, created.ID)
	assert.Equal(t, *created, *repo.tasks[created.ID], "the stored task is the one returned")
}

func TestServiceCreateGeneratesUniqueIDs(t *testing.T) {
	service := newTestService(t, newFakeRepository())
	first, err := service.Create(context.Background(), "org_a", CreateInput{Title: "a"})
	require.NoError(t, err)
	second, err := service.Create(context.Background(), "org_a", CreateInput{Title: "b"})
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID)
}

func TestServiceCreateValidates(t *testing.T) {
	for name, test := range map[string]struct {
		input CreateInput
		field string
	}{
		"missing title":  {CreateInput{}, "title"},
		"blank title":    {CreateInput{Title: " \t\n "}, "title"},
		"title too long": {CreateInput{Title: strings.Repeat("x", 201)}, "title"},
		"notes too long": {CreateInput{Title: "a", Notes: strings.Repeat("x", 2001)}, "notes"},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository()
			_, err := newTestService(t, repo).Create(context.Background(), "org_a", test.input)
			requireInvalidField(t, err, test.field)
			assert.Empty(t, repo.calls, "invalid input never reaches the repository")
		})
	}
}

func TestServiceCreateAcceptsLimits(t *testing.T) {
	service := newTestService(t, newFakeRepository())
	created, err := service.Create(context.Background(), "org_a", CreateInput{
		Title: strings.Repeat("x", 200),
		Notes: strings.Repeat("y", 2000),
	})
	require.NoError(t, err)
	assert.Len(t, created.Title, 200)
	assert.Len(t, created.Notes, 2000)
}

func TestServiceCreateRepositoryError(t *testing.T) {
	repo := newFakeRepository()
	repo.createErr = errors.New("disk full")
	_, err := newTestService(t, repo).Create(context.Background(), "org_a", CreateInput{Title: "a"})
	requireStatus(t, err, http.StatusInternalServerError)
	_, ok := errors.AsType[*response.DatabaseError](err)
	assert.True(t, ok)
	assert.ErrorIs(t, err, repo.createErr, "the cause is kept")
}

func TestServiceGet(t *testing.T) {
	stored := Task{ID: "task_1", OrganisationID: "org_a", Title: "a"}
	service := newTestService(t, newFakeRepository(stored))

	got, err := service.Get(context.Background(), "org_a", "task_1")
	require.NoError(t, err)
	assert.Equal(t, stored, *got)

	_, err = service.Get(context.Background(), "org_b", "task_1")
	requireStatus(t, err, http.StatusNotFound)
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = service.Get(context.Background(), "org_a", "task_missing")
	requireStatus(t, err, http.StatusNotFound)
}

func TestServiceGetRepositoryError(t *testing.T) {
	repo := newFakeRepository()
	repo.getErr = errors.New("connection reset")
	_, err := newTestService(t, repo).Get(context.Background(), "org_a", "task_1")
	requireStatus(t, err, http.StatusInternalServerError)
	_, ok := errors.AsType[*response.DatabaseError](err)
	assert.True(t, ok)
}

func TestServiceListLimits(t *testing.T) {
	for name, test := range map[string]struct {
		limit, wantQueryLimit, wantPageLimit int
	}{
		"default":       {0, defaultLimit + 1, defaultLimit},
		"within range":  {5, 6, 5},
		"maximum":       {maxLimit, maxLimit + 1, maxLimit},
		"above maximum": {maxLimit + 50, maxLimit + 1, maxLimit},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository()
			page, err := newTestService(t, repo).List(context.Background(), "org_a", ListFilter{}, pagination.Params{Limit: test.limit})
			require.NoError(t, err)
			assert.Equal(t, test.wantQueryLimit, repo.lastQuery.Limit, "one extra row tells whether another page follows")
			assert.Equal(t, test.wantPageLimit, page.Meta.Limit)
		})
	}
}

func TestServiceListPassesFilterAndOrganisation(t *testing.T) {
	repo := newFakeRepository()
	_, err := newTestService(t, repo).List(context.Background(), "org_a", ListFilter{Done: ptr(true)}, pagination.Params{})
	require.NoError(t, err)
	assert.Equal(t, "org_a", repo.lastQuery.OrganisationID)
	require.NotNil(t, repo.lastQuery.Done)
	assert.True(t, *repo.lastQuery.Done)
	assert.Nil(t, repo.lastQuery.After, "the first page has no cursor")
}

func TestServiceListDecodesCursor(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	after := pagination.EncodeCursor(pagination.FormatTimeIDCursorValue(createdAt, "task_9"))

	repo := newFakeRepository()
	_, err := newTestService(t, repo).List(context.Background(), "org_a", ListFilter{}, pagination.Params{After: after})
	require.NoError(t, err)
	require.NotNil(t, repo.lastQuery.After)
	assert.True(t, createdAt.Equal(repo.lastQuery.After.CreatedAt))
	assert.Equal(t, "task_9", repo.lastQuery.After.ID)
}

func TestServiceListBuildsPage(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := newFakeRepository()
	repo.listResult = []Task{
		{ID: "task_3", CreatedAt: base.Add(3 * time.Second)},
		{ID: "task_2", CreatedAt: base.Add(2 * time.Second)},
		{ID: "task_1", CreatedAt: base.Add(1 * time.Second)}, // the extra row
	}

	page, err := newTestService(t, repo).List(context.Background(), "org_a", ListFilter{}, pagination.Params{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page.Data, 2, "the extra row is dropped")
	assert.Equal(t, "task_3", page.Data[0].ID)
	assert.Equal(t, "task_2", page.Data[1].ID)
	assert.True(t, page.Meta.HasMore)

	value, err := pagination.DecodeCursor(page.Meta.NextCursor)
	require.NoError(t, err)
	createdAt, id, err := pagination.ParseTimeIDCursorValue(value)
	require.NoError(t, err)
	assert.Equal(t, "task_2", id, "the next page starts after the last task shown")
	assert.True(t, base.Add(2*time.Second).Equal(createdAt))
}

func TestServiceListLastPage(t *testing.T) {
	repo := newFakeRepository()
	repo.listResult = []Task{{ID: "task_1"}}
	page, err := newTestService(t, repo).List(context.Background(), "org_a", ListFilter{}, pagination.Params{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, page.Data, 1)
	assert.False(t, page.Meta.HasMore)
	assert.Empty(t, page.Meta.NextCursor)
}

func TestServiceListEmptyIsNotNil(t *testing.T) {
	page, err := newTestService(t, newFakeRepository()).List(context.Background(), "org_a", ListFilter{}, pagination.Params{})
	require.NoError(t, err)
	assert.NotNil(t, page.Data, "an empty page encodes as [] rather than null")
	assert.Empty(t, page.Data)
}

func TestServiceListRejectsBadPagination(t *testing.T) {
	for name, params := range map[string]pagination.Params{
		"negative limit":       {Limit: -1},
		"cursor not base64":    {After: "!!"},
		"cursor without ID":    {After: pagination.EncodeCursor("2026-01-01T00:00:00Z")},
		"cursor with bad time": {After: pagination.EncodeCursor("yesterday|task_1")},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository()
			_, err := newTestService(t, repo).List(context.Background(), "org_a", ListFilter{}, params)
			requireStatus(t, err, http.StatusBadRequest)
			assert.Empty(t, repo.calls)
		})
	}
}

func TestServiceListRepositoryError(t *testing.T) {
	repo := newFakeRepository()
	repo.listErr = errors.New("timeout")
	_, err := newTestService(t, repo).List(context.Background(), "org_a", ListFilter{}, pagination.Params{})
	requireStatus(t, err, http.StatusInternalServerError)
}

func TestServiceUpdate(t *testing.T) {
	createdAt := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	stored := Task{ID: "task_1", OrganisationID: "org_a", Title: "old", Notes: "keep me", CreatedAt: createdAt, UpdatedAt: createdAt}

	for name, test := range map[string]struct {
		input UpdateInput
		want  func(task *Task)
	}{
		"title only":  {UpdateInput{Title: ptr("  new  ")}, func(task *Task) { task.Title = "new" }},
		"notes only":  {UpdateInput{Notes: ptr(" changed ")}, func(task *Task) { task.Notes = "changed" }},
		"clear notes": {UpdateInput{Notes: ptr("   ")}, func(task *Task) { task.Notes = "" }},
		"done only":   {UpdateInput{Done: ptr(true)}, func(task *Task) { task.Done = true }},
		"everything": {UpdateInput{Title: ptr("t"), Notes: ptr("n"), Done: ptr(true)}, func(task *Task) {
			task.Title, task.Notes, task.Done = "t", "n", true
		}},
		"nothing": {UpdateInput{}, func(*Task) {}},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository(stored)
			updated, err := newTestService(t, repo).Update(context.Background(), "org_a", "task_1", test.input)
			require.NoError(t, err)

			want := stored
			test.want(&want)
			want.UpdatedAt = time.Date(2026, 3, 4, 9, 30, 15, 0, time.UTC)
			assert.Equal(t, want, *updated)
			assert.Equal(t, want, *repo.tasks["task_1"], "the change is stored")
			assert.Equal(t, createdAt, updated.CreatedAt, "created_at never changes")
		})
	}
}

func TestServiceUpdateCanReopen(t *testing.T) {
	repo := newFakeRepository(Task{ID: "task_1", OrganisationID: "org_a", Title: "a", Done: true})
	updated, err := newTestService(t, repo).Update(context.Background(), "org_a", "task_1", UpdateInput{Done: ptr(false)})
	require.NoError(t, err)
	assert.False(t, updated.Done)
}

func TestServiceUpdateValidates(t *testing.T) {
	for name, test := range map[string]struct {
		input UpdateInput
		field string
	}{
		"blank title":    {UpdateInput{Title: ptr("   ")}, "title"},
		"title too long": {UpdateInput{Title: ptr(strings.Repeat("x", 201))}, "title"},
		"notes too long": {UpdateInput{Notes: ptr(strings.Repeat("x", 2001))}, "notes"},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository(Task{ID: "task_1", OrganisationID: "org_a", Title: "a"})
			_, err := newTestService(t, repo).Update(context.Background(), "org_a", "task_1", test.input)
			requireInvalidField(t, err, test.field)
			assert.Empty(t, repo.calls)
		})
	}
}

func TestServiceUpdateNotFound(t *testing.T) {
	repo := newFakeRepository(Task{ID: "task_1", OrganisationID: "org_a", Title: "a"})
	_, err := newTestService(t, repo).Update(context.Background(), "org_b", "task_1", UpdateInput{Done: ptr(true)})
	requireStatus(t, err, http.StatusNotFound)
	assert.Equal(t, []string{"Get"}, repo.calls, "nothing is written for another organisation's task")
	assert.False(t, repo.tasks["task_1"].Done)
}

func TestServiceUpdateDeletedMeanwhile(t *testing.T) {
	repo := newFakeRepository(Task{ID: "task_1", OrganisationID: "org_a", Title: "a"})
	repo.updateErr = ErrNotFound
	_, err := newTestService(t, repo).Update(context.Background(), "org_a", "task_1", UpdateInput{Done: ptr(true)})
	requireStatus(t, err, http.StatusNotFound)
}

func TestServiceUpdateRepositoryErrors(t *testing.T) {
	for name, setup := range map[string]func(*fakeRepository){
		"get fails":    func(r *fakeRepository) { r.getErr = errors.New("boom") },
		"update fails": func(r *fakeRepository) { r.updateErr = errors.New("boom") },
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository(Task{ID: "task_1", OrganisationID: "org_a", Title: "a"})
			setup(repo)
			_, err := newTestService(t, repo).Update(context.Background(), "org_a", "task_1", UpdateInput{Done: ptr(true)})
			requireStatus(t, err, http.StatusInternalServerError)
		})
	}
}

func TestServiceDelete(t *testing.T) {
	repo := newFakeRepository(Task{ID: "task_1", OrganisationID: "org_a"})
	service := newTestService(t, repo)

	requireStatus(t, service.Delete(context.Background(), "org_b", "task_1"), http.StatusNotFound)
	assert.Contains(t, repo.tasks, "task_1", "another organisation cannot delete it")

	require.NoError(t, service.Delete(context.Background(), "org_a", "task_1"))
	assert.NotContains(t, repo.tasks, "task_1")

	requireStatus(t, service.Delete(context.Background(), "org_a", "task_1"), http.StatusNotFound)
}

func TestServiceDeleteRepositoryError(t *testing.T) {
	repo := newFakeRepository()
	repo.deleteErr = errors.New("locked")
	err := newTestService(t, repo).Delete(context.Background(), "org_a", "task_1")
	requireStatus(t, err, http.StatusInternalServerError)
}
