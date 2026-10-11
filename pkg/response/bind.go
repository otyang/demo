package response

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strings"

	"uradical.io/go/binder"

	"github.com/otyang/demo/pkg/validation"
)

// MaxBodyBytes is the largest request body Bind reads.
const MaxBodyBytes = 1 << 20

// Bind binds the request into dst with binder and maps its failures to
// response errors, using the statuses binder suggests. A body must be sent as
// JSON, and a top-level key no field binds is rejected. binder's own messages
// stay in the cause, which is logged, not sent: they name Go fields.
func Bind(r *http.Request, dst any) error {
	if err := requireJSON(r); err != nil {
		return err
	}
	err := binder.BindWithOptions(r, dst, binder.BindOptions{
		MaxBodySize:           MaxBodyBytes,
		DisallowUnknownFields: true,
	})
	if err == nil {
		return nil
	}
	if errs, ok := errors.AsType[binder.BindErrors](err); ok {
		fields := make(validation.Errors, 0, len(errs))
		for _, e := range errs {
			fields = append(fields, fieldError(e))
		}
		return NewBadRequestError("Invalid request", fields, err)
	}
	switch {
	case errors.Is(err, binder.ErrBodyTooLarge):
		return payloadTooLarge(err)
	case errors.Is(err, binder.ErrMalformedBody):
		return NewBadRequestError("Malformed request body", nil, err)
	case errors.Is(err, binder.ErrInvalidTarget):
		// A handler bug, not the client's fault.
		return NewInternalServerError(err)
	}
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return payloadTooLarge(err)
	}
	// The body could not be read, such as when the client disconnected.
	return NewBadRequestError("Request body could not be read", nil, err)
}

// BindQueryStrict is Bind for a request read from its query, such as a list
// filter, that also rejects a query key dst has no query field for and extra
// does not name (for example "limit" and "after", which handlers read
// themselves). binder only rejects unknown body keys, so without this a
// misspelt filter is ignored and the list silently widens.
func BindQueryStrict(r *http.Request, dst any, extra ...string) error {
	known := queryKeys(reflect.TypeOf(dst), extra)
	var unknown []string
	for key := range r.URL.Query() {
		if !slices.Contains(known, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		fields := make(validation.Errors, 0, len(unknown))
		for _, key := range unknown {
			fields = append(fields, validation.FieldError{Field: key, Tag: "unknown", Message: key + " is not a known query parameter"})
		}
		return NewBadRequestError("Invalid request", fields, nil)
	}
	return Bind(r, dst)
}

// queryKeys returns the query tag names of t's fields, including those of
// embedded structs, plus extra.
func queryKeys(t reflect.Type, extra []string) []string {
	keys := slices.Clone(extra)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return keys
	}
	for field := range t.Fields() {
		if name, _, _ := strings.Cut(field.Tag.Get("query"), ","); name != "" && name != "-" {
			keys = append(keys, name)
		} else if field.Anonymous {
			keys = append(keys, queryKeys(field.Type, nil)...)
		}
	}
	return keys
}

// requireJSON rejects a body not sent as JSON: binder would otherwise bind
// nothing from it without an error, or parse it as a form. A request without
// a body, such as a GET bound from its query, needs no Content-Type.
func requireJSON(r *http.Request) error {
	if r.ContentLength == 0 {
		return nil
	}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		return nil
	}
	return NewError(http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json", nil, nil)
}

// fieldError describes one binder field failure in the same shape as a
// validation error, named by the key the client sent.
func fieldError(e *binder.BindError) validation.FieldError {
	switch {
	case errors.Is(e, binder.ErrUnknownField):
		return validation.FieldError{Field: e.Name, Tag: "unknown", Message: e.Name + " is not a known field"}
	case errors.Is(e, binder.ErrMissingRequired):
		return validation.FieldError{Field: e.Name, Tag: "required", Message: e.Name + " is required"}
	default:
		return validation.FieldError{Field: e.Name, Tag: "type", Message: e.Name + " has an invalid value"}
	}
}

func payloadTooLarge(err error) error {
	return NewError(
		http.StatusRequestEntityTooLarge,
		"PAYLOAD_TOO_LARGE",
		fmt.Sprintf("payload too large (limit %d bytes)", MaxBodyBytes),
		nil,
		err,
	)
}
