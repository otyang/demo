package response

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/otyang/demo/pkg/pagination"
)

type Envelope struct {
	Status bool  `json:"status"`
	Data   any   `json:"data"`
	Meta   *Meta `json:"meta,omitempty"`
}

type Meta = pagination.Meta

type ErrorEnvelope struct {
	Status bool     `json:"status"`
	Error  APIError `json:"error"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func Success(data any) Envelope {
	return Envelope{Status: true, Data: data}
}

// WriteJSON writes body as JSON with the given status. Characters such as &,
// < and > are written as-is rather than as \u0026-style escapes, so URLs in
// responses (for example an AuthKit authorization URL) can be copied and used
// directly. The escaping only matters when JSON is embedded in HTML, which
// this API never does.
func WriteJSON(w http.ResponseWriter, status int, body any) error {
	if body == nil {
		w.WriteHeader(status)
		return nil
	}

	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(body); err != nil { // Encode appends a newline
		return fmt.Errorf("marshal API response: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(encoded.Bytes()); err != nil {
		return fmt.Errorf("write API response: %w", err)
	}
	return nil
}

func WriteSuccess(w http.ResponseWriter, status int, data any) error {
	return WriteJSON(w, status, Success(data))
}

func WriteListCursor(w http.ResponseWriter, status int, data any, meta pagination.Meta) error {
	return WriteJSON(w, status, Envelope{
		Status: true,
		Data:   data,
		Meta:   &meta,
	})
}

func WriteError(w http.ResponseWriter, status int, code, message string, details ...any) error {
	return WriteJSON(w, status, failure(code, message, details...))
}

func failure(code, message string, details ...any) ErrorEnvelope {
	var detail any
	if len(details) > 0 {
		detail = details[0]
	}
	return ErrorEnvelope{
		Status: false,
		Error: APIError{
			Code:    code,
			Message: message,
			Details: detail,
		},
	}
}
