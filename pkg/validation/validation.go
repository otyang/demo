package validation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/locales"
	"github.com/go-playground/locales/de"
	"github.com/go-playground/locales/en"
	"github.com/go-playground/locales/es"
	"github.com/go-playground/locales/fr"
	"github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	deTranslations "github.com/go-playground/validator/v10/translations/de"
	enTranslations "github.com/go-playground/validator/v10/translations/en"
	esTranslations "github.com/go-playground/validator/v10/translations/es"
	frTranslations "github.com/go-playground/validator/v10/translations/fr"
)

type CustomRule struct {
	Tag       string
	Validate  validator.Func
	Translate string
}

type Options struct {
	Locale      string
	TagNameFunc validator.TagNameFunc
	CustomRules []CustomRule
}

type Validator struct {
	validate   *validator.Validate
	translator ut.Translator
}

type FieldError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

type Errors []FieldError

var defaultValidator = mustNewDefault()

func (e Errors) Error() string {
	if len(e) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed: %s", e[0].Message)
}

func New(options Options) (*Validator, error) {
	locale := strings.ToLower(strings.TrimSpace(options.Locale))
	if locale == "" {
		locale = "en"
	}

	localizer, registerTranslations, err := localeFor(locale)
	if err != nil {
		return nil, err
	}

	uni := ut.New(localizer, localizer)
	translator, found := uni.GetTranslator(locale)
	if !found {
		return nil, fmt.Errorf("create translator for locale %q", locale)
	}

	validate := validator.New()
	tagNameFunc := options.TagNameFunc
	if tagNameFunc == nil {
		tagNameFunc = RequestTagName
	}
	validate.RegisterTagNameFunc(tagNameFunc)
	for _, rule := range options.CustomRules {
		if rule.Tag == "" {
			return nil, fmt.Errorf("custom validation tag cannot be empty")
		}
		if rule.Validate == nil {
			return nil, fmt.Errorf("custom validation %q has no validator function", rule.Tag)
		}
		if err := validate.RegisterValidation(rule.Tag, rule.Validate); err != nil {
			return nil, fmt.Errorf("register custom validation %q: %w", rule.Tag, err)
		}
	}
	if err := registerTranslations(validate, translator); err != nil {
		return nil, fmt.Errorf("register %s translations: %w", locale, err)
	}
	for _, rule := range options.CustomRules {
		if rule.Translate == "" {
			return nil, fmt.Errorf("custom validation %q has no translation", rule.Tag)
		}
		translation := rule.Translate
		if err := validate.RegisterTranslation(rule.Tag, translator, func(ut.Translator) error {
			template := strings.ReplaceAll(translation, "{field}", "{0}")
			template = strings.ReplaceAll(template, "{param}", "{1}")
			return translator.Add(rule.Tag, template, false)
		}, func(translator ut.Translator, fieldErr validator.FieldError) string {
			message, err := translator.T(rule.Tag, fieldErr.Field(), fieldErr.Param())
			if err != nil {
				return translation
			}
			return message
		}); err != nil {
			return nil, fmt.Errorf("register translation for custom validation %q: %w", rule.Tag, err)
		}
	}

	return &Validator{validate: validate, translator: translator}, nil
}

func Default() *Validator {
	return defaultValidator
}

func Validate(value any) error {
	return defaultValidator.Validate(value)
}

func (v *Validator) Validate(value any) error {
	err := v.validate.Struct(value)
	if err == nil {
		return nil
	}

	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return fmt.Errorf("validate struct: %w", err)
	}

	result := make(Errors, 0, len(validationErrors))
	for _, fieldErr := range validationErrors {
		result = append(result, FieldError{
			Field:   fieldPath(fieldErr),
			Tag:     fieldErr.Tag(),
			Message: translatedMessage(v.translator, fieldErr),
		})
	}
	return result
}

// fieldPath names a failed field by its path from the validated struct, such
// as metadata.website for a field of a nested struct, as binder names its own
// failures.
func fieldPath(fieldErr validator.FieldError) string {
	if _, path, ok := strings.Cut(fieldErr.Namespace(), "."); ok {
		return path
	}
	return fieldErr.Field()
}

func mustNewDefault() *Validator {
	v, err := New(Options{Locale: "en", TagNameFunc: RequestTagName})
	if err != nil {
		panic(fmt.Sprintf("initialize default validator: %v", err))
	}
	return v
}

// RequestTagName names a field by the key a client sent it under: its
// uradical.io/go/binder tag (body, query or path), or else its json tag, as
// on nested structs that are also stored or returned as JSON.
func RequestTagName(field reflect.StructField) string {
	for _, tag := range []string{"body", "query", "path"} {
		if name := strings.SplitN(field.Tag.Get(tag), ",", 2)[0]; name != "" {
			return name
		}
	}
	return JSONTagName(field)
}

func JSONTagName(field reflect.StructField) string {
	name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
	if name == "-" {
		return ""
	}
	return name
}

func localeFor(locale string) (locales.Translator, func(*validator.Validate, ut.Translator) error, error) {
	switch locale {
	case "de":
		return de.New(), deTranslations.RegisterDefaultTranslations, nil
	case "en":
		return en.New(), enTranslations.RegisterDefaultTranslations, nil
	case "es":
		return es.New(), esTranslations.RegisterDefaultTranslations, nil
	case "fr":
		return fr.New(), frTranslations.RegisterDefaultTranslations, nil
	default:
		return nil, nil, fmt.Errorf("unsupported translation locale %q; supported locales: de, en, es, fr", locale)
	}
}

func translatedMessage(translator ut.Translator, fieldErr validator.FieldError) string {
	return fieldErr.Translate(translator)
}
