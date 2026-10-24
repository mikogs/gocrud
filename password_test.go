package gocrud

import (
	"context"
	"errors"
	"testing"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type PasswordStruct struct {
	ID       uint64
	Name     string `validate:"required,min=1,max=100"`
	Password string `crud:"pass"`
}

func recreatePasswordStructTable() {
	_ = testCRUD.DropTable(context.Background(), &PasswordStruct{})
	_ = testCRUD.CreateTable(context.Background(), &PasswordStruct{})
}

func TestLoad_PasswordFieldIsZeroed(t *testing.T) {
	recreatePasswordStructTable()

	obj := &PasswordStruct{Name: "alice", Password: "secret123"}
	if err := testCRUD.Save(context.Background(), obj, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	loaded := &PasswordStruct{}
	if err := testCRUD.Load(context.Background(), loaded, obj.ID, LoadOptions{}); err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}

	if loaded.Password != "" {
		t.Errorf("Load: expected password field to be empty, got %q", loaded.Password)
	}
	if loaded.Name != "alice" {
		t.Errorf("Load: expected Name to be %q, got %q", "alice", loaded.Name)
	}
}

func TestGet_PasswordFieldIsZeroed(t *testing.T) {
	recreatePasswordStructTable()

	for _, name := range []string{"bob", "carol"} {
		obj := &PasswordStruct{Name: name, Password: "hunter2"}
		if err := testCRUD.Save(context.Background(), obj, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	results, err := testCRUD.Get(context.Background(), func() any {
		return &PasswordStruct{}
	}, GetOptions{})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}

	if len(results) != 2 {
		t.Fatalf("Get: expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		ps := r.(*PasswordStruct)
		if ps.Password != "" {
			t.Errorf("Get: expected password field to be empty for %q, got %q", ps.Name, ps.Password)
		}
	}
}

func TestVerifyPasswords(t *testing.T) {
	recreatePasswordStructTable()
	ctx := context.Background()

	obj := &PasswordStruct{Name: "erin", Password: "correct-horse"}
	if err := testCRUD.Save(ctx, obj, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	byName := sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "erin"}}

	verifiedObj := &PasswordStruct{}
	verified, err := testCRUD.VerifyPasswords(ctx, verifiedObj, VerifyPasswordsOptions{
		Filters:   byName,
		Passwords: map[string]string{"Password": "correct-horse"},
	})
	if err != nil || !verified || verifiedObj.ID != obj.ID || verifiedObj.Name != "erin" || verifiedObj.Password != "" {
		t.Fatalf("VerifyPasswords of the password returned %t, %v, %+v, want the object without its password", verified, err, *verifiedObj)
	}

	wrongObj := &PasswordStruct{}
	verified, err = testCRUD.VerifyPasswords(ctx, wrongObj, VerifyPasswordsOptions{
		Filters:   byName,
		Passwords: map[string]string{"Password": "wrong-password"},
	})
	if err != nil || verified || *wrongObj != (PasswordStruct{}) {
		t.Fatalf("VerifyPasswords of another password returned %t, %v, %+v, want false and an empty object", verified, err, *wrongObj)
	}

	verified, err = testCRUD.VerifyPasswords(ctx, &PasswordStruct{}, VerifyPasswordsOptions{
		Filters:                  sqlfilters.Filters{"ID": {Op: sqlfilters.OpEqual, Val: "1"}},
		ConvertFiltersFromString: true,
		Passwords:                map[string]string{"Password": "correct-horse"},
	})
	if err != nil || !verified {
		t.Fatalf("VerifyPasswords with filters from strings returned %t, %v, want true", verified, err)
	}

	for name, passwords := range map[string]map[string]string{
		"no passwords":            {},
		"a field without a hash":  {"Name": "erin"},
		"a field that is missing": {"Unknown": "x"},
	} {
		if verified, err := testCRUD.VerifyPasswords(ctx, &PasswordStruct{}, VerifyPasswordsOptions{Filters: byName, Passwords: passwords}); !errors.Is(err, ErrVerifyPasswords) || verified {
			t.Fatalf("VerifyPasswords with %s returned %t, %v, want ErrVerifyPasswords", name, verified, err)
		}
	}
}

type TwoPasswords struct {
	ID       uint64
	Name     string
	Password string `crud:"pass"`
	Pin      string `crud:"pass"`
}

func TestVerifyPasswords_All(t *testing.T) {
	ctx := context.Background()
	crud := testCRUDSQLite
	_ = DropTable[TwoPasswords](ctx, crud)
	if err := CreateTable[TwoPasswords](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	if err := crud.Save(ctx, &TwoPasswords{Name: "a", Password: "secret", Pin: "1234"}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	// the passwords are checked in the random order of the map, so each case is checked more than once
	for i := 0; i < 8; i++ {
		for _, test := range []struct {
			passwords map[string]string
			want      bool
		}{
			{map[string]string{"Password": "secret", "Pin": "1234"}, true},
			{map[string]string{"Password": "wrong", "Pin": "1234"}, false},
			{map[string]string{"Password": "secret", "Pin": "0000"}, false},
		} {
			verified, err := crud.VerifyPasswords(ctx, &TwoPasswords{}, VerifyPasswordsOptions{
				Filters:   sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "a"}},
				Passwords: test.passwords,
			})
			if err != nil || verified != test.want {
				t.Fatalf("VerifyPasswords of %v returned %t, %v, want %t", test.passwords, verified, err, test.want)
			}
		}
	}
}
