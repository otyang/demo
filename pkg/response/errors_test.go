package response

import (
	"errors"
	"net/http"
	"testing"
)

func TestTypedErrorConstructors(t *testing.T) {
	tests := []struct {
		name   string
		err    ResponseError
		status int
		code   string
	}{
		{"bad request", NewBadRequestError("Invalid request", nil, nil), http.StatusBadRequest, "BAD_REQUEST"},
		{"unauthorized", NewUnauthorizedError("Authentication required", nil), http.StatusUnauthorized, "UNAUTHORIZED"},
		{"forbidden", NewForbiddenError("Access denied", nil), http.StatusForbidden, "FORBIDDEN"},
		{"validation", NewValidationError(nil, nil), http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{"not found", NewNotFoundError("Missing", nil), http.StatusNotFound, "NOT_FOUND"},
		{"conflict", NewConflictError("Resource already exists", nil, nil), http.StatusConflict, "CONFLICT"},
		{"database", NewDatabaseError(nil), http.StatusInternalServerError, "DATABASE_ERROR"},
		{"internal server", NewInternalServerError(nil), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.HTTPStatus(); got != test.status {
				t.Errorf("HTTPStatus() = %d, want %d", got, test.status)
			}
			if got := test.err.ErrorPayload().Code; got != test.code {
				t.Errorf("ErrorPayload().Code = %q, want %q", got, test.code)
			}
			if test.err.Error() == "" {
				t.Error("Error() returned an empty message")
			}
		})
	}
}

func TestNewForbiddenErrorWithCode(t *testing.T) {
	err := NewForbiddenErrorWithCode("ORG_SETUP_REQUIRED", "Organization is required", nil)
	if got := err.HTTPStatus(); got != http.StatusForbidden {
		t.Errorf("HTTPStatus() = %d, want %d", got, http.StatusForbidden)
	}
	if got := err.ErrorPayload().Code; got != "ORG_SETUP_REQUIRED" {
		t.Errorf("ErrorPayload().Code = %q, want ORG_SETUP_REQUIRED", got)
	}
}

func TestNewErrorSupportsOtherHTTPStatuses(t *testing.T) {
	err := NewError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
	if err.HTTPStatus() != http.StatusMethodNotAllowed {
		t.Errorf("HTTPStatus() = %d, want %d", err.HTTPStatus(), http.StatusMethodNotAllowed)
	}
	if err.ErrorPayload().Code != "METHOD_NOT_ALLOWED" {
		t.Errorf("ErrorPayload().Code = %q, want METHOD_NOT_ALLOWED", err.ErrorPayload().Code)
	}
}

func TestAppErrorWrapsCause(t *testing.T) {
	cause := errors.New("database connection failed")
	err := NewDatabaseError(cause)

	if !errors.Is(err, cause) {
		t.Fatal("errors.Is() did not find the original cause")
	}
	if got, want := err.Error(), "DATABASE_ERROR: database connection failed"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
