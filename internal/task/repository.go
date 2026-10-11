package task

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/otyang/demo/pkg/store"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate creates or updates the tasks table. It records applied
// migrations in task_migrations, apart from other services'.
func Migrate(ctx context.Context, db *bun.DB, logger *slog.Logger) error {
	return store.MigrateEmbedded(ctx, db, migrations, "migrations", store.MigrationOptions{
		Logger:    logger,
		TableName: "task_migrations",
	})
}

// ErrNotFound reports that no task has the ID in the organisation.
var ErrNotFound = errors.New("task not found")

// Cursor is the position after which a page of tasks starts: the last task
// of the previous page.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// ListQuery selects a page of an organisation's tasks, newest first.
type ListQuery struct {
	OrganisationID string
	Done           *bool
	After          *Cursor
	Limit          int
}

// Repository stores tasks. Every method is scoped to one organisation.
type Repository interface {
	Create(ctx context.Context, task *Task) error
	Get(ctx context.Context, organisationID, id string) (*Task, error)
	List(ctx context.Context, query ListQuery) ([]Task, error)
	Update(ctx context.Context, task *Task) error
	Delete(ctx context.Context, organisationID, id string) error
}

// BunRepository is the Repository backed by Bun, on SQLite or PostgreSQL.
type BunRepository struct {
	db bun.IDB
}

// NewBunRepository returns a Repository using db.
func NewBunRepository(db bun.IDB) (*BunRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("task repository: database cannot be nil")
	}
	return &BunRepository{db: db}, nil
}

func (r *BunRepository) Create(ctx context.Context, task *Task) error {
	if _, err := r.db.NewInsert().Model(task).Exec(ctx); err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	return nil
}

func (r *BunRepository) Get(ctx context.Context, organisationID, id string) (*Task, error) {
	var task Task
	err := r.db.NewSelect().Model(&task).
		Where("organisation_id = ?", organisationID).
		Where("id = ?", id).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select task: %w", err)
	}
	return &task, nil
}

func (r *BunRepository) List(ctx context.Context, query ListQuery) ([]Task, error) {
	tasks := make([]Task, 0, query.Limit)
	selection := r.db.NewSelect().Model(&tasks).
		Where("organisation_id = ?", query.OrganisationID).
		OrderExpr("created_at DESC, id DESC").
		Limit(int64(query.Limit))
	if query.Done != nil {
		selection = selection.Where("done = ?", *query.Done)
	}
	if after := query.After; after != nil {
		selection = selection.Where("(created_at < ? OR (created_at = ? AND id < ?))", after.CreatedAt, after.CreatedAt, after.ID)
	}
	if err := selection.Scan(ctx); err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return tasks, nil
}

func (r *BunRepository) Update(ctx context.Context, task *Task) error {
	result, err := r.db.NewUpdate().Model(task).
		Column("title", "notes", "done", "updated_at").
		Where("organisation_id = ?", task.OrganisationID).
		WherePK().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return requireRow(result)
}

func (r *BunRepository) Delete(ctx context.Context, organisationID, id string) error {
	result, err := r.db.NewDelete().Model((*Task)(nil)).
		Where("organisation_id = ?", organisationID).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return requireRow(result)
}

// requireRow returns ErrNotFound when a statement changed no row.
func requireRow(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count affected rows: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
