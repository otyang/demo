package task

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otyang/demo/pkg/store"
)

// newTestRepository returns a BunRepository on a fresh, migrated in-memory
// SQLite database.
func newTestRepository(t *testing.T) *BunRepository {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := store.New(store.Options{
		Type: store.SQLite, DSN: "file:repo_" + name + "?mode=memory&cache=shared", MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, Migrate(context.Background(), db.BunDB, slog.New(slog.DiscardHandler)))

	repo, err := NewBunRepository(db.BunDB)
	require.NoError(t, err)
	return repo
}

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// seed stores a task created seconds after epoch.
func seed(t *testing.T, repo *BunRepository, org, id string, seconds int, done bool) Task {
	t.Helper()
	at := epoch.Add(time.Duration(seconds) * time.Second)
	task := Task{ID: id, OrganisationID: org, Title: "title " + id, Notes: "notes " + id, Done: done, CreatedAt: at, UpdatedAt: at}
	require.NoError(t, repo.Create(context.Background(), &task))
	return task
}

// assertSameTask compares tasks, treating equal instants in any zone as
// equal.
func assertSameTask(t *testing.T, want, got Task) {
	t.Helper()
	assert.True(t, want.CreatedAt.Equal(got.CreatedAt), "created_at: want %v, got %v", want.CreatedAt, got.CreatedAt)
	assert.True(t, want.UpdatedAt.Equal(got.UpdatedAt), "updated_at: want %v, got %v", want.UpdatedAt, got.UpdatedAt)
	want.CreatedAt, want.UpdatedAt, got.CreatedAt, got.UpdatedAt = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	assert.Equal(t, want, got)
}

func taskIDs(tasks []Task) []string {
	result := make([]string, len(tasks))
	for i, task := range tasks {
		result[i] = task.ID
	}
	return result
}

func TestNewBunRepositoryRequiresDatabase(t *testing.T) {
	_, err := NewBunRepository(nil)
	assert.Error(t, err)
}

func TestMigrateIsRepeatable(t *testing.T) {
	db, err := store.New(store.Options{Type: store.SQLite, DSN: "file:repo_migrate?mode=memory&cache=shared", MaxOpenConns: 1, MaxIdleConns: 1})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	logger := slog.New(slog.DiscardHandler)
	require.NoError(t, Migrate(context.Background(), db.BunDB, logger))
	require.NoError(t, Migrate(context.Background(), db.BunDB, logger), "applied migrations are skipped")
}

func TestRepositoryCreateAndGet(t *testing.T) {
	repo := newTestRepository(t)
	stored := seed(t, repo, "org_a", "task_1", 0, true)

	got, err := repo.Get(context.Background(), "org_a", "task_1")
	require.NoError(t, err)
	assertSameTask(t, stored, *got)
}

func TestRepositoryCreateRejectsDuplicateID(t *testing.T) {
	repo := newTestRepository(t)
	seed(t, repo, "org_a", "task_1", 0, false)

	duplicate := Task{ID: "task_1", OrganisationID: "org_b", Title: "again", CreatedAt: epoch, UpdatedAt: epoch}
	err := repo.Create(context.Background(), &duplicate)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
}

func TestRepositoryGetNotFound(t *testing.T) {
	repo := newTestRepository(t)
	seed(t, repo, "org_a", "task_1", 0, false)

	_, err := repo.Get(context.Background(), "org_a", "task_missing")
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = repo.Get(context.Background(), "org_b", "task_1")
	assert.ErrorIs(t, err, ErrNotFound, "another organisation's task is not found")
}

func TestRepositoryListNewestFirst(t *testing.T) {
	repo := newTestRepository(t)
	seed(t, repo, "org_a", "task_b", 1, false)
	seed(t, repo, "org_a", "task_c", 2, false)
	seed(t, repo, "org_a", "task_a", 1, false) // same second as task_b
	seed(t, repo, "org_b", "task_z", 3, false)

	tasks, err := repo.List(context.Background(), ListQuery{OrganisationID: "org_a", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{"task_c", "task_b", "task_a"}, taskIDs(tasks), "newest first, then by ID descending; org_b is excluded")
}

func TestRepositoryListLimit(t *testing.T) {
	repo := newTestRepository(t)
	for i, id := range []string{"task_1", "task_2", "task_3"} {
		seed(t, repo, "org_a", id, i, false)
	}

	tasks, err := repo.List(context.Background(), ListQuery{OrganisationID: "org_a", Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"task_3", "task_2"}, taskIDs(tasks))
}

func TestRepositoryListFiltersByDone(t *testing.T) {
	repo := newTestRepository(t)
	seed(t, repo, "org_a", "task_open", 0, false)
	seed(t, repo, "org_a", "task_done", 1, true)

	for name, test := range map[string]struct {
		done *bool
		want []string
	}{
		"any":  {nil, []string{"task_done", "task_open"}},
		"done": {ptr(true), []string{"task_done"}},
		"open": {ptr(false), []string{"task_open"}},
	} {
		t.Run(name, func(t *testing.T) {
			tasks, err := repo.List(context.Background(), ListQuery{OrganisationID: "org_a", Done: test.done, Limit: 10})
			require.NoError(t, err)
			assert.Equal(t, test.want, taskIDs(tasks))
		})
	}
}

func TestRepositoryListAfterCursor(t *testing.T) {
	repo := newTestRepository(t)
	seed(t, repo, "org_a", "task_a", 1, false)
	seed(t, repo, "org_a", "task_b", 1, false)
	seed(t, repo, "org_a", "task_c", 1, false)
	seed(t, repo, "org_a", "task_d", 2, false)
	seed(t, repo, "org_a", "task_e", 0, false)

	for name, test := range map[string]struct {
		after Cursor
		want  []string
	}{
		"after the newest":           {Cursor{epoch.Add(2 * time.Second), "task_d"}, []string{"task_c", "task_b", "task_a", "task_e"}},
		"within a shared second":     {Cursor{epoch.Add(1 * time.Second), "task_b"}, []string{"task_a", "task_e"}},
		"after the last of a second": {Cursor{epoch.Add(1 * time.Second), "task_a"}, []string{"task_e"}},
		"after the oldest":           {Cursor{epoch, "task_e"}, []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			after := test.after
			tasks, err := repo.List(context.Background(), ListQuery{OrganisationID: "org_a", After: &after, Limit: 10})
			require.NoError(t, err)
			assert.Equal(t, test.want, taskIDs(tasks))
		})
	}
}

func TestRepositoryListPagesThroughEverything(t *testing.T) {
	repo := newTestRepository(t)
	var want []string
	for i := range 7 {
		id := "task_" + string(rune('a'+i))
		seed(t, repo, "org_a", id, i/3, false) // three tasks share each second
		want = append(want, id)
	}

	var seen []string
	query := ListQuery{OrganisationID: "org_a", Limit: 2}
	for pages := 0; ; pages++ {
		require.Less(t, pages, 10, "paging did not end")
		tasks, err := repo.List(context.Background(), query)
		require.NoError(t, err)
		if len(tasks) == 0 {
			break
		}
		seen = append(seen, taskIDs(tasks)...)
		last := tasks[len(tasks)-1]
		query.After = &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	assert.ElementsMatch(t, want, seen)
	assert.Len(t, seen, len(want), "no task is listed twice")
}

func TestRepositoryListEmpty(t *testing.T) {
	repo := newTestRepository(t)
	tasks, err := repo.List(context.Background(), ListQuery{OrganisationID: "org_a", Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, tasks)
	assert.Empty(t, tasks)
}

func TestRepositoryUpdate(t *testing.T) {
	repo := newTestRepository(t)
	stored := seed(t, repo, "org_a", "task_1", 0, false)

	changed := stored
	changed.Title, changed.Notes, changed.Done = "new title", "new notes", true
	changed.UpdatedAt = epoch.Add(time.Hour)
	changed.CreatedAt = epoch.Add(24 * time.Hour) // not an updatable column
	require.NoError(t, repo.Update(context.Background(), &changed))

	got, err := repo.Get(context.Background(), "org_a", "task_1")
	require.NoError(t, err)
	want := changed
	want.CreatedAt = stored.CreatedAt
	assertSameTask(t, want, *got)
}

func TestRepositoryUpdateNotFound(t *testing.T) {
	repo := newTestRepository(t)
	stored := seed(t, repo, "org_a", "task_1", 0, false)

	missing := stored
	missing.ID = "task_missing"
	assert.ErrorIs(t, repo.Update(context.Background(), &missing), ErrNotFound)

	foreign := stored
	foreign.OrganisationID = "org_b"
	foreign.Title = "hijacked"
	assert.ErrorIs(t, repo.Update(context.Background(), &foreign), ErrNotFound, "another organisation cannot update it")

	got, err := repo.Get(context.Background(), "org_a", "task_1")
	require.NoError(t, err)
	assert.Equal(t, stored.Title, got.Title)
}

func TestRepositoryDelete(t *testing.T) {
	repo := newTestRepository(t)
	seed(t, repo, "org_a", "task_1", 0, false)
	seed(t, repo, "org_a", "task_2", 1, false)

	assert.ErrorIs(t, repo.Delete(context.Background(), "org_b", "task_1"), ErrNotFound, "another organisation cannot delete it")

	require.NoError(t, repo.Delete(context.Background(), "org_a", "task_1"))
	_, err := repo.Get(context.Background(), "org_a", "task_1")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = repo.Get(context.Background(), "org_a", "task_2")
	assert.NoError(t, err, "other tasks are kept")

	assert.ErrorIs(t, repo.Delete(context.Background(), "org_a", "task_1"), ErrNotFound, "deleting twice is not found")
}

func TestRepositoryReturnsErrorsAfterClose(t *testing.T) {
	db, err := store.New(store.Options{Type: store.SQLite, DSN: "file:repo_closed?mode=memory&cache=shared", MaxOpenConns: 1, MaxIdleConns: 1})
	require.NoError(t, err)
	require.NoError(t, Migrate(context.Background(), db.BunDB, slog.New(slog.DiscardHandler)))
	repo, err := NewBunRepository(db.BunDB)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	ctx := context.Background()
	task := Task{ID: "task_1", OrganisationID: "org_a", Title: "a", CreatedAt: epoch, UpdatedAt: epoch}
	for name, call := range map[string]func() error{
		"create": func() error { return repo.Create(ctx, &task) },
		"get":    func() error { _, err := repo.Get(ctx, "org_a", "task_1"); return err },
		"list":   func() error { _, err := repo.List(ctx, ListQuery{OrganisationID: "org_a", Limit: 1}); return err },
		"update": func() error { return repo.Update(ctx, &task) },
		"delete": func() error { return repo.Delete(ctx, "org_a", "task_1") },
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrNotFound, "a failure is not mistaken for a missing task")
		})
	}
}
