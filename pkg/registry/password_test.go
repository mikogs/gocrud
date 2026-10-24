package registry

import (
	"context"
	"errors"
	"reflect"
	"strconv"
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

func TestVerifyPasswords(t *testing.T) {
	recreatePasswordStructTable()
	ctx := context.Background()

	obj := &PasswordStruct{Name: "erin", Password: "correct-horse"}
	if err := testCRUD.Save(ctx, obj, gocrud.SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	byName := Filters{Vals: map[string]string{"Name": "erin"}}

	verifiedObj, verified, err := testService.VerifyPasswords(ctx, "passwordstruct", byName, map[string]string{"Password": "correct-horse"})
	if err != nil || !verified || verifiedObj.(*PasswordStruct).ID != obj.ID || verifiedObj.(*PasswordStruct).Password != "" {
		t.Fatalf("VerifyPasswords of the password returned %v, %t, %v, want the record without its password", verifiedObj, verified, err)
	}
	byID := Filters{Vals: map[string]string{"ID": strconv.FormatUint(obj.ID, 10)}}
	if _, verified, err := testService.VerifyPasswords(ctx, "passwordstruct", byID, map[string]string{"Password": "correct-horse"}); err != nil || !verified {
		t.Fatalf("VerifyPasswords by the ID, as a string, returned %t, %v, want true", verified, err)
	}

	for name, test := range map[string]struct {
		key       string
		filters   Filters
		passwords map[string]string
		wantErr   error
	}{
		"another password": {"passwordstruct", byName, map[string]string{"Password": "wrong-password"}, nil},
		"no record":        {"passwordstruct", Filters{Vals: map[string]string{"Name": "nobody"}}, map[string]string{"Password": "correct-horse"}, nil},
		"an unknown key":   {"unknown", byName, map[string]string{"Password": "correct-horse"}, ErrInvalidKey},
		"no passwords":     {"passwordstruct", byName, map[string]string{}, gocrud.ErrVerifyPasswords},
	} {
		obj, verified, err := testService.VerifyPasswords(ctx, test.key, test.filters, test.passwords)
		if obj != nil || verified || !errors.Is(err, test.wantErr) || test.wantErr == nil && err != nil {
			t.Fatalf("VerifyPasswords with %s returned %v, %t, %v, want nil, false and %v", name, obj, verified, err, test.wantErr)
		}
	}

	_, _, err = testService.VerifyPasswords(ctx, "passwordstruct", Filters{Vals: map[string]string{"ID": "x"}}, map[string]string{"Password": "correct-horse"})
	var filterErr *FilterValidationError
	if !errors.As(err, &filterErr) || filterErr.Violations["ID"] != gocrud.ViolationType {
		t.Fatalf("VerifyPasswords with an invalid filter returned %v, want a FilterValidationError", err)
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

	_, err := testService.List(context.Background(), "passwordstruct", ListOptions{Limit: 10, Filters: Filters{Vals: map[string]string{"Password": "$2a$%"}, Ops: map[string]string{"Password": "like"}}})

	var validationErr *FilterValidationError
	if !errors.As(err, &validationErr) || !reflect.DeepEqual(validationErr.Violations, map[string]string{"Password": gocrud.ViolationPassword}) {
		t.Fatalf("List with a filter on a password returned %v, want %v with the violation %s", err, &FilterValidationError{}, gocrud.ViolationPassword)
	}

	_, err = testService.List(context.Background(), "passwordstruct", ListOptions{Limit: 10, Order: "Password", OrderDirection: "asc"})
	if !errors.As(err, &validationErr) {
		t.Fatalf("List ordered by a password returned %v, want %v", err, &FilterValidationError{})
	}
}
