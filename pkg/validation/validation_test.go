package validation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestValidateRequiredAndJSONFieldNames(t *testing.T) {
	v, err := New(Options{Locale: "en", TagNameFunc: JSONTagName})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	input := struct {
		FirstName string `json:"firstName" validate:"required"`
	}{}
	err = v.Validate(input)
	if err == nil {
		t.Fatal("Validate() error = nil, want required field error")
	}

	var fields Errors
	if !asErrors(err, &fields) {
		t.Fatalf("error type = %T, want validation.Errors", err)
	}
	if len(fields) != 1 {
		t.Fatalf("len(errors) = %d, want 1", len(fields))
	}
	if fields[0].Field != "firstName" {
		t.Errorf("Field = %q, want %q", fields[0].Field, "firstName")
	}
	if fields[0].Tag != "required" {
		t.Errorf("Tag = %q, want %q", fields[0].Tag, "required")
	}
	if fields[0].Message == "" {
		t.Error("Message is empty")
	}
}

func TestValidateTranslatedLocale(t *testing.T) {
	v, err := New(Options{Locale: "fr", TagNameFunc: JSONTagName})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	input := struct {
		Name string `json:"name" validate:"required"`
	}{}
	err = v.Validate(input)
	if err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
	var fields Errors
	if !asErrors(err, &fields) {
		t.Fatalf("error type = %T, want validation.Errors", err)
	}
	if len(fields) != 1 || fields[0].Field != "name" {
		t.Fatalf("validation errors = %#v, want one translated name field error", fields)
	}
	if !strings.Contains(strings.ToLower(fields[0].Message), "obligatoire") {
		t.Errorf("French message = %q, want French required-field translation", fields[0].Message)
	}
}

func TestCustomValidationRuleAndTranslation(t *testing.T) {
	v, err := New(Options{
		Locale:      "en",
		TagNameFunc: JSONTagName,
		CustomRules: []CustomRule{
			{
				Tag: "notblank",
				Validate: func(field validator.FieldLevel) bool {
					return strings.TrimSpace(field.Field().String()) != ""
				},
				Translate: "The {field} field must not be blank ({param})",
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	input := struct {
		DisplayName string `json:"displayName" validate:"notblank=trimmed"`
	}{DisplayName: "   "}
	err = v.Validate(input)
	if err == nil {
		t.Fatal("Validate() error = nil, want custom validation error")
	}
	var fields Errors
	if !asErrors(err, &fields) {
		t.Fatalf("error type = %T, want validation.Errors", err)
	}
	if len(fields) != 1 {
		t.Fatalf("len(errors) = %d, want 1", len(fields))
	}
	if fields[0].Message != "The displayName field must not be blank (trimmed)" {
		t.Errorf("Message = %q, want custom translation with field and parameter", fields[0].Message)
	}
}

func TestValidatePassesValidStruct(t *testing.T) {
	v, err := New(Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := struct {
		Name string `validate:"required"`
	}{Name: "Ada"}
	if err := v.Validate(input); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestPackageValidateUsesInitializedDefault(t *testing.T) {
	first := Default()
	second := Default()
	if first != second {
		t.Fatal("Default() returned different validator instances")
	}

	input := struct {
		FirstName string `json:"firstName" validate:"required"`
	}{}
	err := Validate(input)
	if err == nil {
		t.Fatal("Validate() error = nil, want required field error")
	}
	var fields Errors
	if !asErrors(err, &fields) {
		t.Fatalf("error type = %T, want validation.Errors", err)
	}
	if len(fields) != 1 || fields[0].Field != "firstName" {
		t.Errorf("default validation errors = %#v, want JSON field name firstName", fields)
	}
}

func TestNewRejectsUnsupportedLocaleAndInvalidCustomRule(t *testing.T) {
	if _, err := New(Options{Locale: "xx"}); err == nil {
		t.Error("New() error = nil for unsupported locale")
	}
	if _, err := New(Options{CustomRules: []CustomRule{{Tag: "custom"}}}); err == nil {
		t.Error("New() error = nil for custom rule without validator")
	}
	if _, err := New(Options{CustomRules: []CustomRule{{Validate: func(validator.FieldLevel) bool { return true }, Translate: "invalid"}}}); err == nil {
		t.Error("New() error = nil for custom rule without tag")
	}
	if _, err := New(Options{CustomRules: []CustomRule{{Tag: "custom", Validate: func(validator.FieldLevel) bool { return true }}}}); err == nil {
		t.Error("New() error = nil for custom rule without translation")
	}
}

func asErrors(err error, target *Errors) bool {
	errors, ok := err.(Errors)
	if !ok {
		return false
	}
	*target = errors
	return true
}

func TestRequestTagNameUsesBindingTags(t *testing.T) {
	type input struct {
		Body   string `body:"bodyName" json:"ignored"`
		Query  string `query:"queryName,omitempty"`
		Path   string `path:"pathName"`
		JSON   string `json:"jsonName,omitempty"`
		Hidden string `json:"-"`
		Plain  string
	}
	want := map[string]string{"Body": "bodyName", "Query": "queryName", "Path": "pathName", "JSON": "jsonName", "Hidden": "", "Plain": ""}
	typ := reflect.TypeFor[input]()
	for i := range typ.NumField() {
		field := typ.Field(i)
		if got := RequestTagName(field); got != want[field.Name] {
			t.Errorf("RequestTagName(%s) = %q, want %q", field.Name, got, want[field.Name])
		}
	}
}

func TestDefaultValidatorNamesFieldsByBindingTag(t *testing.T) {
	type input struct {
		Email string `body:"email" validate:"required"`
	}
	var fields Errors
	if err := Validate(input{}); !asErrors(err, &fields) || len(fields) != 1 || fields[0].Field != "email" {
		t.Fatalf("Validate() = %v, want one error for field email", err)
	}
}

func TestDefaultValidatorNamesNestedFieldsByPath(t *testing.T) {
	type metadata struct {
		Website *string `body:"website" validate:"omitnil,max=3"`
	}
	type input struct {
		Metadata metadata `body:"metadata"`
	}
	website := "https://example.com"
	var fields Errors
	if err := Validate(&input{Metadata: metadata{Website: &website}}); !asErrors(err, &fields) || len(fields) != 1 || fields[0].Field != "metadata.website" {
		t.Fatalf("Validate() = %v, want one error for field metadata.website", err)
	}
}
