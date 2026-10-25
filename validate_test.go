package gocrud

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/mikogs/gocrud/internal/fixtures"
)

type ValidationTestStruct struct {
	ID             uint64
	Flags          uint64
	PrimaryEmail   string `validate:"required,email"`
	EmailSecondary string `validate:"required,email"`
	FirstName      string `validate:"required,min=2,max=30"`
	LastName       string `validate:"required,max=255"`
	Age            int    `validate:"min=1,max=120"`
	Price          int    `validate:"min=0,max=999"`
	PostCode       string `validate:"required,min=6" crud_regexp:"^[0-9]{2}\\-[0-9]{3}$"`
	PostCode2      string `validate:"min=6" crud_regexp:"^[0-9]{2}\\-[0-9]{3}$"`
	Password       string
	CreatedBy      uint64
	Key            string `crud:"uniq" validate:"required,min=30,max=255"`
}

func validationTestStructWithData() *ValidationTestStruct {
	ts := &ValidationTestStruct{}
	ts.Flags = 4
	ts.PrimaryEmail = "primary@example.com"
	ts.EmailSecondary = "secondary@example.com"
	ts.FirstName = "John"
	ts.LastName = "Smith"
	ts.Age = 37
	ts.Price = 444
	ts.PostCode = "00-000"
	ts.PostCode2 = "11-111"
	ts.Password = "yyy"
	ts.CreatedBy = 4
	ts.Key = fmt.Sprintf("12345679012345678901234567890%d", time.Now().UnixNano())

	return ts
}

func TestValidate_WithValidStruct(t *testing.T) {
	ts := validationTestStructWithData()
	ok, violations, err := Validate(ts, nil, "crud")
	if !ok {
		t.Fatal("Validate failed validate valid struct")
	}
	if len(violations) > 0 {
		t.Fatal("Validate return non-empty failed field list when validating a valid struct")
	}
	if err != nil {
		t.Fatalf("Validate failed to validate valid struct: %s", err.Error())
	}
}

func TestValidateWithValidStructAndListOfFields(t *testing.T) {
	ts := validationTestStructWithData()
	ts.Age = 0
	ts.FirstName = "x"
	ts.Key = "tooshort"
	ts.PrimaryEmail = "thisis@valid.email.com"
	ok, violations, err := Validate(ts, map[string]bool{
		"PrimaryEmail": true,
		"Price":        true,
	}, "crud")
	if !ok {
		t.Fatal("Validate failed to validate listed fields")
	}
	if len(violations) > 0 {
		t.Fatal("Validate return non-empty failed field list when validating listed fields")
	}
	if err != nil {
		t.Fatalf("Validate failed to validate listed fields: %s", err.Error())
	}
}

func TestValidate_WithInvalidStruct(t *testing.T) {
	ts := validationTestStructWithData()
	ts.PrimaryEmail = "invalidemail"
	ts.EmailSecondary = "invalidemail"
	ts.FirstName = "x"
	ts.LastName = "aFbdsZFYxMpUNKCkBrHhhODrMBEHtmRAJjoqSSfUotvsfMXcJGPrCRaDOsyuyrXYfACjsJEMUoxNvTwRMUaWYruOxgzTXJRzobmxaFbdsZFYxMpUNKCkBrHhhODrMBEHtmRAJjoqSSfUotvsfMXcJGPrCRaDOsyuyrXYfACjsJEMUoxNvTwRMUaWYruOxgzTXJRzobmxaFbdsZFYxMpUNKCkBrHhhODrMBEHtmRAJjoqSSfUotvsfMXcJGPrCRaDOsyuyrXYfACjsJEMUoxNvTwRMUaWYruOxgzTXJRzobmxaFbdsZFYxMpUNKCkBrHhhODrMBEHtmRAJjoqSSfUotvsfMXcJGPrCRaDOsyuyrXYfACjsJEMUoxNvTwRMUaWYruOxgzTXJRzobmx"
	ts.Age = 0
	ts.Price = 1000
	ts.PostCode = "inv"
	ts.PostCode2 = "inv"
	ts.Key = "tooshort"
	ok, violations, err := Validate(ts, nil, "crud")
	if err != nil {
		t.Fatal("Validate failed with an err")
	}
	if ok {
		t.Fatal("Validate failed to return false for struct with invalid field values")
	}
	for _, f := range []string{"PrimaryEmail", "EmailSecondary", "FirstName", "LastName", "Age", "Price", "PostCode", "PostCode2", "Key"} {
		if violations[f] == "" {
			t.Fatalf("Validate failed to return field %s in failed fields", f)
		}
	}
}

func TestValidate_ViolationTags(t *testing.T) {
	ts := validationTestStructWithData()
	ts.PrimaryEmail = ""
	ts.EmailSecondary = "invalidemail"
	ts.FirstName = "x"
	ts.Price = 1000
	ts.PostCode2 = "abcdef"
	_, violations, err := Validate(ts, nil, "crud")
	if err != nil {
		t.Fatalf("Validate failed with an err: %s", err.Error())
	}

	want := map[string]string{
		"PrimaryEmail":   "required",
		"EmailSecondary": "email",
		"FirstName":      "min",
		"Price":          "max",
		"PostCode2":      ViolationRegExp,
	}
	if len(violations) != len(want) {
		t.Fatalf("Validate returned %v, want %v", violations, want)
	}
	for f, tag := range want {
		if violations[f] != tag {
			t.Fatalf("Validate returned %q for field %s, want %q", violations[f], f, tag)
		}
	}
}

func TestValidateValues(t *testing.T) {
	ts := validationTestStructWithData()
	ok, violations, err := ValidateValues(ts, map[string]any{
		"FirstName": "x",
		"Age":       "notanint",
		"Price":     500,
	}, "crud")
	if err != nil {
		t.Fatalf("ValidateValues failed with an err: %s", err.Error())
	}
	if ok {
		t.Fatal("ValidateValues failed to return false for invalid values")
	}
	if len(violations) != 2 || violations["FirstName"] != "min" || violations["Age"] != ViolationType {
		t.Fatalf("ValidateValues returned unexpected violations: %v", violations)
	}
	if ts.FirstName != "John" || ts.Price != 444 {
		t.Fatal("ValidateValues modified the object")
	}
}

func invalidTestStruct() (*fixtures.TestStruct, map[string]string) {
	ts := fixtures.TestStructWithData()
	ts.EmailSecondary = "invalidemail"
	ts.FirstName = "x"
	ts.PostCode2 = "abcdef"

	return ts, map[string]string{
		"EmailSecondary": "email",
		"FirstName":      "min",
		"PostCode2":      ViolationRegExp,
	}
}

func fatalIfNotViolations(t *testing.T, err error, want map[string]string) {
	t.Helper()

	var crudErr *CRUDError
	if !errors.As(err, &crudErr) {
		t.Fatalf("want *CRUDError, got %v", err)
	}

	var validationErr *ValidationError
	if !errors.As(crudErr.Err, &validationErr) {
		t.Fatalf("want *ValidationError, got %v", crudErr.Err)
	}

	if !reflect.DeepEqual(validationErr.Violations, want) {
		t.Fatalf("got violations %v, want %v", validationErr.Violations, want)
	}
}

type validationEdgeCasesTestStruct struct {
	InvalidPattern string `crud_regexp:"[a-"`
	Skipped        string `validate:"-" crud_regexp:"^[a-z]+$"`
	Phone          string `validate:"omitempty,min=7" crud_regexp:"^[0-9]+$"`
}

func TestValidate_InvalidRegExpPattern(t *testing.T) {
	_, violations, err := Validate(&validationEdgeCasesTestStruct{InvalidPattern: "abc"}, nil, "crud")
	if err != nil {
		t.Fatalf("Validate failed with an err: %s", err.Error())
	}
	if violations["InvalidPattern"] != ViolationRegExp {
		t.Fatalf("Validate returned %q for an invalid pattern, want %q", violations["InvalidPattern"], ViolationRegExp)
	}
}

func TestValidate_SkippedField(t *testing.T) {
	_, violations, err := Validate(&validationEdgeCasesTestStruct{InvalidPattern: "abc", Skipped: "123"}, map[string]bool{
		"Skipped": true,
	}, "crud")
	if err != nil {
		t.Fatalf("Validate failed with an err: %s", err.Error())
	}
	if len(violations) > 0 {
		t.Fatalf("Validate returned violations %v for a skipped field", violations)
	}
}

func TestValidate_OmitEmpty(t *testing.T) {
	restrictFields := map[string]bool{"Phone": true}

	ok, violations, _ := Validate(&validationEdgeCasesTestStruct{}, restrictFields, "crud")
	if !ok {
		t.Fatalf("Validate returned violations %v for an empty optional field", violations)
	}

	_, violations, _ = Validate(&validationEdgeCasesTestStruct{Phone: "123"}, restrictFields, "crud")
	if violations["Phone"] != "min" {
		t.Fatalf("Validate returned %q for a too short optional field, want %q", violations["Phone"], "min")
	}

	_, violations, _ = Validate(&validationEdgeCasesTestStruct{Phone: "abcdefgh"}, restrictFields, "crud")
	if violations["Phone"] != ViolationRegExp {
		t.Fatalf("Validate returned %q for a non-matching optional field, want %q", violations["Phone"], ViolationRegExp)
	}
}

func TestValidate_InvalidObject(t *testing.T) {
	var nilObj *validationEdgeCasesTestStruct
	for name, obj := range map[string]any{"nil pointer": nilObj, "not a struct": "string"} {
		if _, _, err := Validate(obj, nil, "crud"); err == nil {
			t.Fatalf("Validate did not return an err for %s", name)
		}
		if _, _, err := ValidateValues(obj, map[string]any{"Phone": "1234567"}, "crud"); err == nil {
			t.Fatalf("ValidateValues did not return an err for %s", name)
		}
	}
}

type validationNestedTestStruct struct {
	Name string `validate:"required"`
}

type validationOuterTestStruct struct {
	Nested  validationNestedTestStruct
	Pointer *validationNestedTestStruct
	Skipped validationNestedTestStruct `validate:"-"`
	Created time.Time
}

func TestValidate_NestedStruct(t *testing.T) {
	obj := &validationOuterTestStruct{Pointer: &validationNestedTestStruct{}}
	_, violations, err := Validate(obj, nil, "crud")
	if err != nil {
		t.Fatalf("Validate failed with an err: %s", err.Error())
	}
	if violations["Name"] != "required" || len(violations) != 1 {
		t.Fatalf("Validate returned %v for nested structs with an empty required field, want Name: required", violations)
	}

	_, violations, _ = Validate(obj, map[string]bool{"Created": true}, "crud")
	if len(violations) > 0 {
		t.Fatalf("Validate returned violations %v for a field without rules", violations)
	}
}
