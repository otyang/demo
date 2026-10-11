package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/otyang/demo/pkg/ids"
	"github.com/otyang/demo/pkg/pagination"
	"github.com/otyang/demo/pkg/response"
	"github.com/otyang/demo/pkg/validation"
)

const (
	idPrefix = "task_"
	// defaultLimit and maxLimit bound a page of tasks.
	defaultLimit = 20
	maxLimit     = 100
)

// Service holds the task rules. Its errors are response errors, which
// httpx writes with their status, so handlers return them unchanged.
type Service struct {
	repo Repository
	// now returns the current time; tests replace it.
	now func() time.Time
}

// NewService returns a Service storing tasks in repo.
func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("task service: repository cannot be nil")
	}
	return &Service{repo: repo, now: time.Now}, nil
}

// Create adds a task to the organisation.
func (s *Service) Create(ctx context.Context, organisationID string, input CreateInput) (*Task, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Notes = strings.TrimSpace(input.Notes)
	if err := validate(input); err != nil {
		return nil, err
	}

	now := s.timestamp()
	task := &Task{
		ID:             ids.New(idPrefix),
		OrganisationID: organisationID,
		Title:          input.Title,
		Notes:          input.Notes,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.Create(ctx, task); err != nil {
		return nil, repositoryError(err)
	}
	return task, nil
}

// Get returns one of the organisation's tasks.
func (s *Service) Get(ctx context.Context, organisationID, id string) (*Task, error) {
	task, err := s.repo.Get(ctx, organisationID, id)
	if err != nil {
		return nil, repositoryError(err)
	}
	return task, nil
}

// List returns a page of the organisation's tasks, newest first.
func (s *Service) List(ctx context.Context, organisationID string, filter ListFilter, page pagination.Params) (pagination.Page[Task], error) {
	page, err := pagination.Normalize(page, defaultLimit, maxLimit)
	if err != nil {
		return pagination.Page[Task]{}, response.NewBadRequestError("Invalid pagination", nil, err)
	}
	query := ListQuery{
		OrganisationID: organisationID,
		Done:           filter.Done,
		// One more than the page, so Build can tell whether another follows.
		Limit: page.Limit + 1,
	}
	if page.After != "" {
		value, _ := pagination.DecodeCursor(page.After) // checked by Normalize
		createdAt, id, err := pagination.ParseTimeIDCursorValue(value)
		if err != nil {
			return pagination.Page[Task]{}, response.NewBadRequestError("Invalid pagination", nil, err)
		}
		query.After = &Cursor{CreatedAt: createdAt, ID: id}
	}

	tasks, err := s.repo.List(ctx, query)
	if err != nil {
		return pagination.Page[Task]{}, repositoryError(err)
	}
	result, err := pagination.Build(tasks, page, func(t Task) string {
		return pagination.FormatTimeIDCursorValue(t.CreatedAt, t.ID)
	}, nil)
	if err != nil {
		return pagination.Page[Task]{}, response.NewInternalServerError(err)
	}
	return result, nil
}

// Update changes the fields of input that are set.
func (s *Service) Update(ctx context.Context, organisationID, id string, input UpdateInput) (*Task, error) {
	if input.Title != nil {
		*input.Title = strings.TrimSpace(*input.Title)
	}
	if input.Notes != nil {
		*input.Notes = strings.TrimSpace(*input.Notes)
	}
	if err := validate(input); err != nil {
		return nil, err
	}

	task, err := s.repo.Get(ctx, organisationID, id)
	if err != nil {
		return nil, repositoryError(err)
	}
	if input.Title != nil {
		task.Title = *input.Title
	}
	if input.Notes != nil {
		task.Notes = *input.Notes
	}
	if input.Done != nil {
		task.Done = *input.Done
	}
	task.UpdatedAt = s.timestamp()
	if err := s.repo.Update(ctx, task); err != nil {
		return nil, repositoryError(err)
	}
	return task, nil
}

// Delete removes one of the organisation's tasks.
func (s *Service) Delete(ctx context.Context, organisationID, id string) error {
	return repositoryError(s.repo.Delete(ctx, organisationID, id))
}

// timestamp returns the current time in UTC, truncated to the second. Whole
// seconds store and compare the same way on SQLite (as text) and
// PostgreSQL, which keeps the created_at cursor exact.
func (s *Service) timestamp() time.Time {
	return s.now().UTC().Truncate(time.Second)
}

// validate checks input's validate tags, answering 422 with each failed
// field.
func validate(input any) error {
	err := validation.Validate(input)
	if fields, ok := errors.AsType[validation.Errors](err); ok {
		return response.NewValidationError(fields, err)
	}
	if err != nil {
		return response.NewInternalServerError(err)
	}
	return nil
}

// repositoryError maps a repository error to a response error.
func repositoryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return response.NewNotFoundError("Task not found", err)
	default:
		return response.NewDatabaseError(err)
	}
}
