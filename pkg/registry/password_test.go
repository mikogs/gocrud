package registry

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud"
)

type PasswordStruct struct {
	ID       uint64
	Name     string `validate:"required,min=1,max=100"`
	Password string `crud:"pass"`
}

// PasswordStruct_Touch is a projection of PasswordStruct, without the Password field.
type PasswordStruct_Touch struct {
	ID   uint64
	Name string `validate:"required,min=1,max=100"`
}

func TestList_VerifyPasswordFields(t *testing.T) {
	recreatePasswordStructTable()

	obj := &PasswordStruct{Name: "erin", Password: "correct-horse"}
	if err := testCRUD.Save(context.Background(), obj, gocrud.SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	results, passwordFields, err := testService.List(context.Background(), "passwordstruct", 10, 0, "", "",
		Filters{Vals: map[string]string{"Name": "erin"}}, nil, nil,
		map[string]string{
			"Password": "correct-horse",
			"Name":     "erin",
		})
	if err != nil {
		t.Fatalf("List failed: %s", err.Error())
	}
	if len(results) != 1 {
		t.Fatalf("List: expected 1 result, got %d", len(results))
	}
	if got := passwordFields["Password"]; got != gocrud.PassOK {
		t.Errorf("expected PassOK for correct password, got %d", got)
	}
	if _, ok := passwordFields["Name"]; ok {
		t.Errorf("expected no entry for non-password field, got %d", passwordFields["Name"])
	}
	if ps := results[0].(*PasswordStruct); ps.Password != "" {
		t.Errorf("List: expected password field to be empty, got %q", ps.Password)
	}

	_, passwordFields2, err := testService.List(context.Background(), "passwordstruct", 10, 0, "", "",
		Filters{Vals: map[string]string{"Name": "erin"}}, nil, nil,
		map[string]string{"Password": "wrong-password"})
	if err != nil {
		t.Fatalf("List failed: %s", err.Error())
	}
	if got := passwordFields2["Password"]; got != gocrud.PassInvalid {
		t.Errorf("expected PassInvalid for wrong password, got %d", got)
	}
}

func TestCreateTables_SkipsUnderscoreProjections(t *testing.T) {
	_ = testCRUD.DropTable(context.Background(), &PasswordStruct{})

	if err := testService.CreateTables(context.Background()); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}

	obj := &PasswordStruct{Name: "table-check", Password: "whatever123"}
	if err := testCRUD.Save(context.Background(), obj, gocrud.SaveOptions{}); err != nil {
		t.Fatalf("Save failed, password_struct table is missing columns (projection likely won table creation): %s", err.Error())
	}
}

func TestList_PasswordFilter(t *testing.T) {
	recreatePasswordStructTable()

	_, _, err := testService.List(context.Background(), "passwordstruct", 10, 0, "", "",
		Filters{Vals: map[string]string{"Password": "$2a$%"}, Ops: map[string]string{"Password": "like"}}, nil, nil, nil)

	var validationErr *FilterValidationError
	if !errors.As(err, &validationErr) || !reflect.DeepEqual(validationErr.Violations, map[string]string{"Password": gocrud.ViolationPassword}) {
		t.Fatalf("List with a filter on a password returned %v, want %v with the violation %s", err, &FilterValidationError{}, gocrud.ViolationPassword)
	}

	_, _, err = testService.List(context.Background(), "passwordstruct", 10, 0, "Password", "asc", Filters{}, nil, nil, nil)
	if !errors.As(err, &validationErr) {
		t.Fatalf("List ordered by a password returned %v, want %v", err, &FilterValidationError{})
	}
}
