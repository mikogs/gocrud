package gocrud

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
	"golang.org/x/crypto/bcrypt"
)

type RequiredPasswordStruct struct {
	ID       uint64
	Password string `crud:"pass" validate:"required"`
}

func recreatePasswordTable(t *testing.T, crud *CRUD) {
	t.Helper()
	ctx := context.Background()

	_ = crud.DropTable(ctx, &PasswordStruct{})
	if err := crud.CreateTable(ctx, &PasswordStruct{}); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
}

func savePasswordStruct(t *testing.T, crud *CRUD, obj *PasswordStruct, options SaveOptions) {
	t.Helper()

	if err := crud.Save(context.Background(), obj, options); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
}

func loadPasswordStruct(t *testing.T, crud *CRUD, id uint64) *PasswordStruct {
	t.Helper()

	obj, err := Load[PasswordStruct](context.Background(), crud, id)
	if err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	return obj
}

// fatalIfPasswords checks the result of the verification of each password of the object named name.
func fatalIfPasswords(t *testing.T, crud *CRUD, name string, want map[string]bool) {
	t.Helper()

	for password, result := range want {
		verified, err := crud.VerifyPasswords(context.Background(), &PasswordStruct{}, VerifyPasswordsOptions{
			Filters:   sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: name}},
			Passwords: map[string]string{"Password": password},
		})
		if err != nil || verified != result {
			t.Fatalf("verification of %q of %s returned %t, %v, want %t", password, name, verified, err, result)
		}
	}
}

func testPasswordKeptOnUpdate(t *testing.T, crud *CRUD) {
	recreatePasswordTable(t, crud)

	obj := &PasswordStruct{Name: "alice", Password: "secret"}
	savePasswordStruct(t, crud, obj, SaveOptions{})
	if obj.Password != "" {
		t.Fatalf("Save left %q in the password field, want it empty", obj.Password)
	}

	loaded := loadPasswordStruct(t, crud, obj.ID)
	loaded.Name = "alicia"
	savePasswordStruct(t, crud, loaded, SaveOptions{})
	fatalIfPasswords(t, crud, "alicia", map[string]bool{"secret": true, "": false})

	loaded = loadPasswordStruct(t, crud, obj.ID)
	loaded.Name = "ali"
	savePasswordStruct(t, crud, loaded, SaveOptions{NoInsert: true})
	fatalIfPasswords(t, crud, "ali", map[string]bool{"secret": true, "": false})

	obj.Name = "al"
	savePasswordStruct(t, crud, obj, SaveOptions{})
	fatalIfPasswords(t, crud, "al", map[string]bool{"secret": true})
}

func testPasswordChanged(t *testing.T, crud *CRUD) {
	recreatePasswordTable(t, crud)

	obj := &PasswordStruct{Name: "bob", Password: "secret"}
	savePasswordStruct(t, crud, obj, SaveOptions{})

	loaded := loadPasswordStruct(t, crud, obj.ID)
	loaded.Password = "changed"
	savePasswordStruct(t, crud, loaded, SaveOptions{})
	fatalIfPasswords(t, crud, "bob", map[string]bool{"changed": true, "secret": false})

	loaded.Password = "again"
	savePasswordStruct(t, crud, loaded, SaveOptions{NoInsert: true})
	fatalIfPasswords(t, crud, "bob", map[string]bool{"again": true, "changed": false})
}

func testPasswordEmptyOnInsert(t *testing.T, crud *CRUD) {
	recreatePasswordTable(t, crud)

	savePasswordStruct(t, crud, &PasswordStruct{Name: "carol"}, SaveOptions{})
	fatalIfPasswords(t, crud, "carol", map[string]bool{"": false, "secret": false})

	savePasswordStruct(t, crud, &PasswordStruct{ID: 42, Name: "dan"}, SaveOptions{})
	fatalIfPasswords(t, crud, "dan", map[string]bool{"": false})
}

func testPasswordHashNotAccepted(t *testing.T, crud *CRUD) {
	recreatePasswordTable(t, crud)

	hash, err := bcrypt.GenerateFromPassword([]byte("known"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	savePasswordStruct(t, crud, &PasswordStruct{Name: "eve", Password: string(hash)}, SaveOptions{})
	fatalIfPasswords(t, crud, "eve", map[string]bool{"known": false, string(hash): true})
}

func testPasswordAfterError(t *testing.T, crud *CRUD) {
	recreatePasswordTable(t, crud)

	obj := &PasswordStruct{ID: 7, Name: "frank", Password: "secret"}
	err := crud.Save(context.Background(), obj, SaveOptions{NoInsert: true})
	if !errors.Is(err, ErrNotFound) || obj.Password != "secret" {
		t.Fatalf("Save of an object that does not exist returned %v, and left %q in the password field, want %v and the password", err, obj.Password, ErrNotFound)
	}

	savePasswordStruct(t, crud, obj, SaveOptions{})
	fatalIfPasswords(t, crud, "frank", map[string]bool{"secret": true})
}

func testPasswordRequired(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = crud.DropTable(ctx, &RequiredPasswordStruct{})
	if err := crud.CreateTable(ctx, &RequiredPasswordStruct{}); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	err := crud.Save(ctx, &RequiredPasswordStruct{}, SaveOptions{})
	fatalIfNotViolations(t, err, map[string]string{"Password": "required"})
}

func testPasswordUpdateMultiple(t *testing.T, crud *CRUD) {
	recreatePasswordTable(t, crud)
	ctx := context.Background()

	for _, name := range []string{"gina", "hank"} {
		savePasswordStruct(t, crud, &PasswordStruct{Name: name, Password: "secret"}, SaveOptions{})
	}
	gina := UpdateMultipleOptions{Filters: sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "gina"}}}

	values := map[string]any{"Password": "changed"}
	if err := UpdateMultiple[PasswordStruct](ctx, crud, values, gina); err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	if values["Password"] != "changed" {
		t.Fatalf("UpdateMultiple changed the values to %v, want them as they were", values)
	}
	fatalIfPasswords(t, crud, "gina", map[string]bool{"changed": true, "secret": false})
	fatalIfPasswords(t, crud, "hank", map[string]bool{"secret": true})

	err := UpdateMultiple[PasswordStruct](ctx, crud, map[string]any{"Password": "again"}, UpdateMultipleOptions{
		Filters: gina.Filters, ConvertValuesFromString: true,
	})
	if err != nil {
		t.Fatalf("UpdateMultiple with values from strings failed: %s", err.Error())
	}
	fatalIfPasswords(t, crud, "gina", map[string]bool{"again": true})

	if err := UpdateMultiple[PasswordStruct](ctx, crud, map[string]any{"Name": "gin", "Password": ""}, gina); err != nil {
		t.Fatalf("UpdateMultiple with an empty password failed: %s", err.Error())
	}
	fatalIfPasswords(t, crud, "gin", map[string]bool{"again": true, "": false})

	if err := UpdateMultiple[PasswordStruct](ctx, crud, map[string]any{"Password": ""}, UpdateMultipleOptions{}); err != nil {
		t.Fatalf("UpdateMultiple of only an empty password failed: %s", err.Error())
	}
	fatalIfPasswords(t, crud, "hank", map[string]bool{"secret": true})
}

func TestPassword_KeptOnUpdate(t *testing.T)        { testPasswordKeptOnUpdate(t, testCRUD) }
func TestSQLite_Password_KeptOnUpdate(t *testing.T) { testPasswordKeptOnUpdate(t, testCRUDSQLite) }

func TestPassword_Changed(t *testing.T)        { testPasswordChanged(t, testCRUD) }
func TestSQLite_Password_Changed(t *testing.T) { testPasswordChanged(t, testCRUDSQLite) }

func TestPassword_EmptyOnInsert(t *testing.T)        { testPasswordEmptyOnInsert(t, testCRUD) }
func TestSQLite_Password_EmptyOnInsert(t *testing.T) { testPasswordEmptyOnInsert(t, testCRUDSQLite) }

func TestPassword_HashNotAccepted(t *testing.T) { testPasswordHashNotAccepted(t, testCRUD) }
func TestSQLite_Password_HashNotAccepted(t *testing.T) {
	testPasswordHashNotAccepted(t, testCRUDSQLite)
}

func TestPassword_AfterError(t *testing.T)        { testPasswordAfterError(t, testCRUD) }
func TestSQLite_Password_AfterError(t *testing.T) { testPasswordAfterError(t, testCRUDSQLite) }

func TestPassword_Required(t *testing.T)        { testPasswordRequired(t, testCRUD) }
func TestSQLite_Password_Required(t *testing.T) { testPasswordRequired(t, testCRUDSQLite) }

func TestPassword_UpdateMultiple(t *testing.T)        { testPasswordUpdateMultiple(t, testCRUD) }
func TestSQLite_Password_UpdateMultiple(t *testing.T) { testPasswordUpdateMultiple(t, testCRUDSQLite) }

func testPasswordTooLong(t *testing.T, crud *CRUD) {
	recreatePasswordTable(t, crud)
	ctx := context.Background()
	long := strings.Repeat("x", 73)

	obj := &PasswordStruct{Name: "liam", Password: long}
	fatalIfNotViolations(t, crud.Save(ctx, obj, SaveOptions{}), map[string]string{"Password": "max"})
	if obj.Password != long {
		t.Fatalf("Save of a password that is too long left %q in the password field, want the password", obj.Password)
	}

	savePasswordStruct(t, crud, &PasswordStruct{Name: "liam", Password: strings.Repeat("x", 72)}, SaveOptions{})
	err := UpdateMultiple[PasswordStruct](ctx, crud, map[string]any{"Password": long}, UpdateMultipleOptions{})
	fatalIfNotViolations(t, err, map[string]string{"Password": "max"})
}

func TestPassword_TooLong(t *testing.T)        { testPasswordTooLong(t, testCRUD) }
func TestSQLite_Password_TooLong(t *testing.T) { testPasswordTooLong(t, testCRUDSQLite) }

// testPasswordNotVerified checks that VerifyPasswords returns false, and an empty object, when no object or more than
// one matches the filters, also with the password of each of them.
func testPasswordNotVerified(t *testing.T, crud *CRUD) {
	recreatePasswordTable(t, crud)
	ctx := context.Background()
	for _, name := range []string{"twin", "twin"} {
		savePasswordStruct(t, crud, &PasswordStruct{Name: name, Password: "secret"}, SaveOptions{})
	}

	for _, name := range []string{"nobody", "twin"} {
		obj := &PasswordStruct{ID: 5, Name: "set"}
		verified, err := crud.VerifyPasswords(ctx, obj, VerifyPasswordsOptions{
			Filters:   sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: name}},
			Passwords: map[string]string{"Password": "secret"},
		})
		if err != nil || verified || *obj != (PasswordStruct{}) {
			t.Fatalf("VerifyPasswords of %s returned %t, %v, %+v, want false and an empty object", name, verified, err, *obj)
		}
	}
}

func TestPassword_NotVerified(t *testing.T)        { testPasswordNotVerified(t, testCRUD) }
func TestSQLite_Password_NotVerified(t *testing.T) { testPasswordNotVerified(t, testCRUDSQLite) }
