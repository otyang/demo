package response

import (
	"errors"
	"fmt"
	"net/http"
)

type ResponseError interface {
	error
	HTTPStatus() int
	ErrorPayload() APIError
}

type AppError struct {
	status  int
	payload APIError
	cause   error
}

func (e *AppError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.payload.Code, e.cause)
	}
	return e.payload.Message
}

func (e *AppError) Unwrap() error {
	return e.cause
}

func (e *AppError) HTTPStatus() int {
	return e.status
}

func (e *AppError) ErrorPayload() APIError {
	return e.payload
}

type BadRequestError struct{ *AppError }
type UnauthorizedError struct{ *AppError }
type ForbiddenError struct{ *AppError }
type ValidationError struct{ *AppError }
type NotFoundError struct{ *AppError }
type ConflictError struct{ *AppError }
type DatabaseError struct{ *AppError }
type InternalServerError struct{ *AppError }

func NewBadRequestError(message string, details any, cause error) *BadRequestError {
	return &BadRequestError{NewError(http.StatusBadRequest, "BAD_REQUEST", message, details, cause)}
}

func NewUnauthorizedError(message string, cause error) *UnauthorizedError {
	return &UnauthorizedError{NewError(http.StatusUnauthorized, "UNAUTHORIZED", message, nil, cause)}
}

func NewForbiddenError(message string, cause error) *ForbiddenError {
	return NewForbiddenErrorWithCode("FORBIDDEN", message, cause)
}

func NewForbiddenErrorWithCode(code, message string, cause error) *ForbiddenError {
	return &ForbiddenError{NewError(http.StatusForbidden, code, message, nil, cause)}
}

func NewValidationError(details any, cause error) *ValidationError {
	return &ValidationError{NewError(http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Request validation failed", details, cause)}
}

func NewNotFoundError(message string, cause error) *NotFoundError {
	return &NotFoundError{NewError(http.StatusNotFound, "NOT_FOUND", message, nil, cause)}
}

func NewConflictError(message string, details any, cause error) *ConflictError {
	return &ConflictError{NewError(http.StatusConflict, "CONFLICT", message, details, cause)}
}

func NewDatabaseError(cause error) *DatabaseError {
	return &DatabaseError{NewError(http.StatusInternalServerError, "DATABASE_ERROR", "A database error occurred", nil, cause)}
}

func NewInternalServerError(cause error) *InternalServerError {
	return &InternalServerError{NewError(http.StatusInternalServerError, "INTERNAL_ERROR", "An internal server error occurred", nil, cause)}
}

func NewError(status int, code, message string, details any, cause error) *AppError {
	return &AppError{
		status:  status,
		payload: APIError{Code: code, Message: message, Details: details},
		cause:   cause,
	}
}

func WriteErrorFrom(w http.ResponseWriter, err error) error {
	if err == nil {
		return errors.New("write API error: nil error")
	}
	if responseErr, ok := errors.AsType[ResponseError](err); ok {
		return WriteJSON(w, responseErr.HTTPStatus(), ErrorEnvelope{
			Status: false,
			Error:  responseErr.ErrorPayload(),
		})
	}
	internalErr := NewInternalServerError(err)
	return WriteJSON(w, internalErr.HTTPStatus(), ErrorEnvelope{
		Status: false,
		Error:  internalErr.ErrorPayload(),
	})
}
