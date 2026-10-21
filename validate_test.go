package gocrud

import (
	"fmt"
	"testing"
	"time"
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

// TestValidateWithValidStruct tests if Validate successfully validates object with valid values
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

// TestValidateWithValidStructAndListOfFields tests if Validate successfully validates object with valid values
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

// TestValidateWithInvalidStruct tests if Validate invalidates object with invalid values
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

// TestValidate_ViolationTags tests if Validate returns names of the failed tags
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

// TestValidateValues tests if ValidateValues validates values without modifying the object
func TestValidateValues(t *testing.T) {
	ts := validationTestStructWithData()
	ok, violations, err := ValidateValues(ts, map[string]interface{}{
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
