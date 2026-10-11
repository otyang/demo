package pagination

import (
	"fmt"
	"strings"
	"time"
)

type Params struct {
	Limit int
	After string
}

type Meta struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor,omitempty"`
	TotalCount *int   `json:"totalCount,omitempty"`
}

type Page[T any] struct {
	Data []T  `json:"data"`
	Meta Meta `json:"meta"`
}

// FormatTimeIDCursorValue returns a cursor payload composed of an RFC3339Nano
// timestamp and an ID. Pass the result through EncodeCursor when exposing it
// as an opaque cursor token.
func FormatTimeIDCursorValue(timestamp time.Time, id string) string {
	return timestamp.UTC().Format(time.RFC3339Nano) + "|" + id
}

// ParseTimeIDCursorValue parses the timestamp and ID payload returned by
// FormatTimeIDCursorValue. The input is the decoded value, not the opaque
// base64 cursor token.
func ParseTimeIDCursorValue(value string) (time.Time, string, error) {
	timestampValue, id, found := strings.Cut(value, "|")
	if !found || timestampValue == "" || id == "" {
		return time.Time{}, "", fmt.Errorf("cursor must contain a timestamp and ID")
	}
	timestamp, err := time.Parse(time.RFC3339Nano, timestampValue)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("parse cursor timestamp: %w", err)
	}
	return timestamp, id, nil
}

func Normalize(params Params, defaultLimit, maxLimit int) (Params, error) {
	if defaultLimit <= 0 {
		return Params{}, fmt.Errorf("default limit must be positive")
	}
	if maxLimit < defaultLimit {
		return Params{}, fmt.Errorf("maximum limit must be greater than or equal to default limit")
	}
	if params.Limit < 0 {
		return Params{}, fmt.Errorf("limit cannot be negative")
	}
	if params.Limit == 0 {
		params.Limit = defaultLimit
	}
	if params.Limit > maxLimit {
		params.Limit = maxLimit
	}
	if params.After != "" {
		if _, err := DecodeCursor(params.After); err != nil {
			return Params{}, fmt.Errorf("invalid after cursor: %w", err)
		}
	}
	return params, nil
}

func Build[T any](rows []T, params Params, cursorOf func(T) string, totalCount *int) (Page[T], error) {
	if params.Limit <= 0 {
		return Page[T]{}, fmt.Errorf("limit must be positive")
	}
	if cursorOf == nil {
		return Page[T]{}, fmt.Errorf("cursor function cannot be nil")
	}
	if params.After != "" {
		if _, err := DecodeCursor(params.After); err != nil {
			return Page[T]{}, fmt.Errorf("invalid after cursor: %w", err)
		}
	}

	hasMore := len(rows) > params.Limit
	if hasMore {
		rows = rows[:params.Limit]
	}
	if rows == nil {
		rows = make([]T, 0)
	}

	meta := Meta{
		Limit:      params.Limit,
		HasMore:    hasMore,
		TotalCount: totalCount,
	}
	if hasMore {
		cursor := cursorOf(rows[len(rows)-1])
		if cursor == "" {
			return Page[T]{}, fmt.Errorf("cursor for last item cannot be empty")
		}
		meta.NextCursor = EncodeCursor(cursor)
	}
	return Page[T]{Data: rows, Meta: meta}, nil
}
