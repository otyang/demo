# Validation

`github.com/otyang/demo/pkg/validation` wraps
[go-playground/validator](https://github.com/go-playground/validator) with
translated field errors and optional custom validation rules.

## Default validator

Import the package and call `validation.Validate` directly. The package creates
one shared validator at initialization with English translations, naming
fields by the key a client sends (`RequestTagName`):

```go
import (
	"errors"

	"github.com/otyang/demo/pkg/response"
	"github.com/otyang/demo/pkg/validation"
)

type CreateUserRequest struct {
	Name  string `body:"name" validate:"required,min=2"`
	Email string `body:"email" validate:"required,email"`
}

if err := validation.Validate(request); err != nil {
	var fields validation.Errors
	if errors.As(err, &fields) {
		return response.NewValidationError(fields, err)
	}
	return err
}
```

Use `validation.Default()` to access that same configured instance directly.
It is initialized once and is safe to share between goroutines. Default
validation names a field by its binder tag (`body`, then `query`, then
`path`), else its `json` tag, else the Go field name. A field of a nested
struct is named by its path, such as `metadata.website`, as binder names its
own failures. Use `JSONTagName` in `Options.TagNameFunc` to name fields by
`json` tags only.

## Custom validator

Use `New` when a custom locale or rules are needed. Each call creates an
independent validator, so create it once during application startup and share
it:

```go
validator, err := validation.New(validation.Options{
	Locale:      "fr",
	TagNameFunc: validation.JSONTagName,
})
if err != nil {
	return err
}
if err := validator.Validate(request); err != nil {
	var fields validation.Errors
	if errors.As(err, &fields) {
		// Pass the field errors to the API response error details.
		return response.NewValidationError(fields, err)
	}
	return err
}
```

`Errors` is a slice of `FieldError` values (`field`, `tag`, and translated
`message`) and implements `error`. Submitted field values are intentionally
not included. `JSONTagName` maps validator fields to their `json` field names;
it is also the default field-name mapping.

## Translations

Built-in translations are available for `en` (the default), `de`, `es`, and `fr`:

```go
validator, err := validation.New(validation.Options{
	Locale:      "fr",
	TagNameFunc: validation.JSONTagName,
})
```

Unsupported locale codes return an error at construction time.

## Custom validation

Register custom validation functions and their translated message when
creating the validator. Messages support `{field}` and `{param}` placeholders:

```go
import (
	"errors"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/otyang/demo/pkg/response"
	"github.com/otyang/demo/pkg/validation"
)

validator, err := validation.New(validation.Options{
	Locale: "en",
	CustomRules: []validation.CustomRule{
		{
			Tag: "notblank",
			Validate: func(field validator.FieldLevel) bool {
				return strings.TrimSpace(field.Field().String()) != ""
			},
			Translate: "The {field} field must not be blank",
		},
	},
})
```

Use the custom rule in a struct tag, for example `validate:"required,notblank"`.
Create one configured validator during application startup and share it; avoid
registering rules after validation begins.
