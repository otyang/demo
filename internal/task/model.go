// Package task is the example service: a to-do list per organisation, built
// in three layers.
//
//   - Repository (repository.go) stores tasks with Bun.
//   - Service (service.go) holds the rules: it validates input, generates
//     IDs, paginates, and turns failures into response errors.
//   - Handler (handler.go) declares the HTTP routes as []doc.Route, binds
//     requests and writes responses.
//
// Every task belongs to the organisation in the caller's JWT, so an
// organisation only ever sees its own tasks.
package task

import (
	"time"

	"github.com/uptrace/bun"
)

// Task is a to-do item, both the stored row and the API's response.
type Task struct {
	bun.BaseModel `bun:"table:tasks"`

	ID             string    `bun:"id,pk" json:"id"`
	OrganisationID string    `bun:"organisation_id,notnull" json:"-"`
	Title          string    `bun:"title,notnull" json:"title"`
	Notes          string    `bun:"notes,notnull" json:"notes"`
	Done           bool      `bun:"done,notnull" json:"done"`
	CreatedAt      time.Time `bun:"created_at,notnull" json:"createdAt"`
	UpdatedAt      time.Time `bun:"updated_at,notnull" json:"updatedAt"`
}

// CreateInput is the body of POST /api/v1/tasks. The `body` tags name the
// JSON keys response.Bind reads; the `validate` tags are checked by the
// service with pkg/validation.
type CreateInput struct {
	Title string `body:"title" validate:"required,max=200"`
	Notes string `body:"notes" validate:"max=2000"`
}

// UpdateInput is the body of PATCH /api/v1/tasks/{id}. Only the keys sent
// are changed.
type UpdateInput struct {
	Title *string `body:"title" validate:"omitnil,min=1,max=200"`
	Notes *string `body:"notes" validate:"omitnil,max=2000"`
	Done  *bool   `body:"done"`
}

// ListFilter is the query of GET /api/v1/tasks, besides limit and after.
type ListFilter struct {
	// Done, when set, lists only finished (true) or open (false) tasks.
	Done *bool `query:"done"`
}
