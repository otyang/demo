package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/otyang/demo/pkg/pagination"
)

func TestWriteSuccess(t *testing.T) {
	recorder := httptest.NewRecorder()
	body := struct {
		Message string `json:"message"`
	}{Message: "Hello, Ada!"}

	if err := WriteSuccess(recorder, http.StatusOK, body); err != nil {
		t.Fatalf("WriteSuccess() error = %v", err)
	}
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got, want := recorder.Header().Get("Content-Type"), "application/json"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	assertJSON(t, recorder.Body.Bytes(), `{"status":true,"data":{"message":"Hello, Ada!"}}`)
}

func TestWriteJSONWithNilBodyWritesStatusAndNoBody(t *testing.T) {
	recorder := httptest.NewRecorder()

	if err := WriteJSON(recorder, http.StatusNoContent, nil); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if recorder.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "" {
		t.Errorf("Content-Type = %q, want empty for bodyless response", contentType)
	}
}

func TestWriteListCursor(t *testing.T) {
	recorder := httptest.NewRecorder()
	items := []string{"Ada", "Grace"}
	total := 12
	meta := pagination.Meta{Limit: 10, HasMore: true, NextCursor: "next", TotalCount: &total}

	if err := WriteListCursor(recorder, http.StatusOK, items, meta); err != nil {
		t.Fatalf("WriteListCursor() error = %v", err)
	}
	assertJSON(t, recorder.Body.Bytes(), `{"status":true,"data":["Ada","Grace"],"meta":{"limit":10,"hasMore":true,"nextCursor":"next","totalCount":12}}`)
}

func TestWriteError(t *testing.T) {
	recorder := httptest.NewRecorder()
	details := map[string]string{"field": "email"}

	if err := WriteError(recorder, http.StatusBadRequest, "INVALID_INPUT", "Request validation failed", details); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	assertJSON(t, recorder.Body.Bytes(), `{"status":false,"error":{"code":"INVALID_INPUT","message":"Request validation failed","details":{"field":"email"}}}`)
}

func TestWriteJSONDoesNotEscapeURLs(t *testing.T) {
	recorder := httptest.NewRecorder()
	url := "https://api.workos.com/user_management/authorize?client_id=client_1&provider=authkit&state=<x>"

	if err := WriteSuccess(recorder, http.StatusOK, map[string]string{"authorizationUrl": url}); err != nil {
		t.Fatalf("WriteSuccess() error = %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, url) {
		t.Errorf("body = %s, want the URL unescaped so it can be copied", body)
	}
	if strings.Contains(body, `\u0026`) {
		t.Errorf("body = %s, contains a \\u0026 escape", body)
	}
	if !strings.HasSuffix(body, "}\n") {
		t.Errorf("body = %q, want a trailing newline", body)
	}
}

func TestFailureDetailsAreOptional(t *testing.T) {
	assertJSON(t, mustMarshal(t, failure("NOT_FOUND", "Resource not found")), `{"status":false,"error":{"code":"NOT_FOUND","message":"Resource not found"}}`)
	assertJSON(t, mustMarshal(t, failure("INVALID_INPUT", "Invalid request", map[string]string{"field": "name"})), `{"status":false,"error":{"code":"INVALID_INPUT","message":"Invalid request","details":{"field":"name"}}}`)

	recorder := httptest.NewRecorder()
	if err := WriteError(recorder, http.StatusNotFound, "NOT_FOUND", "Resource not found"); err != nil {
		t.Fatalf("WriteError() without details error = %v", err)
	}
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	assertJSON(t, recorder.Body.Bytes(), `{"status":false,"error":{"code":"NOT_FOUND","message":"Resource not found"}}`)
}

func TestWriteJSONMarshalFailureDoesNotWriteResponse(t *testing.T) {
	recorder := httptest.NewRecorder()

	err := WriteJSON(recorder, http.StatusOK, map[string]any{"invalid": make(chan int)})
	if err == nil {
		t.Fatal("WriteJSON() error = nil, want marshal error")
	}
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want implicit status %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("body = %q, want empty body", recorder.Body.String())
	}
}

type failingResponseWriter struct {
	header http.Header
}

func (w *failingResponseWriter) Header() http.Header {
	return w.header
}

func (*failingResponseWriter) WriteHeader(int) {}

func (*failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestWriteJSONReturnsWriteError(t *testing.T) {
	writer := &failingResponseWriter{header: make(http.Header)}

	if err := WriteSuccess(writer, http.StatusOK, "ok"); err == nil {
		t.Fatal("WriteSuccess() error = nil, want write error")
	}
}

func TestWriteErrorFrom(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		status     int
		body       string
		hiddenText string
	}{
		{
			name:   "bad request",
			err:    NewBadRequestError("Malformed request", nil, nil),
			status: http.StatusBadRequest,
			body:   `{"status":false,"error":{"code":"BAD_REQUEST","message":"Malformed request"}}`,
		},
		{
			name:   "validation",
			err:    NewValidationError(map[string]string{"email": "invalid"}, nil),
			status: http.StatusUnprocessableEntity,
			body:   `{"status":false,"error":{"code":"VALIDATION_ERROR","message":"Request validation failed","details":{"email":"invalid"}}}`,
		},
		{
			name:   "not found through wrapping",
			err:    errors.Join(errors.New("lookup user"), NewNotFoundError("User not found", nil)),
			status: http.StatusNotFound,
			body:   `{"status":false,"error":{"code":"NOT_FOUND","message":"User not found"}}`,
		},
		{
			name:   "conflict",
			err:    NewConflictError("Resource already exists", map[string]string{"field": "email"}, nil),
			status: http.StatusConflict,
			body:   `{"status":false,"error":{"code":"CONFLICT","message":"Resource already exists","details":{"field":"email"}}}`,
		},
		{
			name:       "database hides cause",
			err:        NewDatabaseError(errors.New("password leaked by driver")),
			status:     http.StatusInternalServerError,
			body:       `{"status":false,"error":{"code":"DATABASE_ERROR","message":"A database error occurred"}}`,
			hiddenText: "password leaked by driver",
		},
		{
			name:       "unknown error is internal",
			err:        errors.New("private implementation detail"),
			status:     http.StatusInternalServerError,
			body:       `{"status":false,"error":{"code":"INTERNAL_ERROR","message":"An internal server error occurred"}}`,
			hiddenText: "private implementation detail",
		},
		{
			name:   "typed internal error",
			err:    NewInternalServerError(errors.New("private implementation detail")),
			status: http.StatusInternalServerError,
			body:   `{"status":false,"error":{"code":"INTERNAL_ERROR","message":"An internal server error occurred"}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			if err := WriteErrorFrom(recorder, test.err); err != nil {
				t.Fatalf("WriteErrorFrom() error = %v", err)
			}
			if recorder.Code != test.status {
				t.Errorf("status = %d, want %d", recorder.Code, test.status)
			}
			assertJSON(t, recorder.Body.Bytes(), test.body)
			if test.hiddenText != "" && strings.Contains(recorder.Body.String(), test.hiddenText) {
				t.Errorf("response leaked internal text %q", test.hiddenText)
			}
		})
	}
}

func TestTypedErrorsCanBeInspectedAndUnwrapped(t *testing.T) {
	cause := errors.New("database connection refused")
	err := NewDatabaseError(cause)

	var databaseErr *DatabaseError
	if !errors.As(err, &databaseErr) {
		t.Fatal("errors.As() did not find DatabaseError")
	}
	if !errors.Is(err, cause) {
		t.Error("errors.Is() did not find wrapped cause")
	}
}

func TestWriteErrorFromRejectsNilError(t *testing.T) {
	if err := WriteErrorFrom(httptest.NewRecorder(), nil); err == nil {
		t.Fatal("WriteErrorFrom() error = nil, want nil-input error")
	}
}

func assertJSON(t *testing.T, got []byte, want string) {
	t.Helper()

	var actual any
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	var expected any
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatalf("invalid expected JSON: %v", err)
	}
	if !equalJSON(actual, expected) {
		t.Errorf("response JSON = %s, want %s", got, want)
	}
}

func equalJSON(a, b any) bool {
	aJSON, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bJSON, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	return encoded
}
