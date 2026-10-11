package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObservabilityRecoversPanicsWithSafeJSON(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := Observability(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("sensitive internal detail")
	}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body["status"] != false {
		t.Errorf("body status = %v, want false", body["status"])
	}
	if strings.Contains(recorder.Body.String(), "sensitive internal detail") {
		t.Error("response leaked panic value")
	}
	if !strings.Contains(logs.String(), "sensitive internal detail") {
		t.Error("panic detail was not written to internal logs")
	}
	if !strings.Contains(logs.String(), "HTTP request") {
		t.Error("request access log was not written")
	}
}

func TestStatusWriterRecordsImplicitOK(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &statusWriter{ResponseWriter: recorder}

	if _, err := writer.Write([]byte("ok")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if writer.status != http.StatusOK {
		t.Errorf("status = %d, want %d", writer.status, http.StatusOK)
	}
}

func TestObservabilityLetsErrAbortHandlerThrough(t *testing.T) {
	handler := Observability(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
}
