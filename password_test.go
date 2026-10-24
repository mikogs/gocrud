package gocrud

import (
	"context"
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
	if _, err := testCRUD.Load(context.Background(), loaded, obj.ID, LoadOptions{}); err != nil {
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

	results, _, err := testCRUD.Get(context.Background(), func() interface{} {
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

func TestLoad_VerifyPasswordFields(t *testing.T) {
	recreatePasswordStructTable()

	obj := &PasswordStruct{Name: "dave", Password: "correct-horse"}
	if err := testCRUD.Save(context.Background(), obj, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	loaded := &PasswordStruct{}
	passwordFields, err := testCRUD.Load(context.Background(), loaded, obj.ID, LoadOptions{
		VerifyPasswordFields: map[string]string{
			"Password": "correct-horse",
			"Name":     "dave",
		},
	})
	if err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}

	if got := passwordFields["Password"]; got != PassOK {
		t.Errorf("expected PassOK for correct password, got %d", got)
	}
	if _, ok := passwordFields["Name"]; ok {
		t.Errorf("expected no entry for non-password field, got %d", passwordFields["Name"])
	}
	if len(passwordFields) != 1 {
		t.Errorf("expected exactly 1 entry in PasswordFields, got %d", len(passwordFields))
	}

	loaded2 := &PasswordStruct{}
	passwordFields2, err := testCRUD.Load(context.Background(), loaded2, obj.ID, LoadOptions{
		VerifyPasswordFields: map[string]string{
			"Password": "wrong-password",
		},
	})
	if err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if got := passwordFields2["Password"]; got != PassInvalid {
		t.Errorf("expected PassInvalid for wrong password, got %d", got)
	}

	if loaded2.Password != "" {
		t.Errorf("Load: expected password field to be empty, got %q", loaded2.Password)
	}
}

func TestGet_SingleResultWithoutVerifyPasswordFields(t *testing.T) {
	recreatePasswordStructTable()

	obj := &PasswordStruct{Name: "gina", Password: "correct-horse"}
	if err := testCRUD.Save(context.Background(), obj, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	results, passwordFields, err := testCRUD.Get(context.Background(), func() interface{} {
		return &PasswordStruct{}
	}, GetOptions{
		Filters: &sqlfilters.Filters{
			"Name": {Op: sqlfilters.OpEqual, Val: "gina"},
		},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if len(results) != 1 {
		t.Fatalf("Get: expected 1 result, got %d", len(results))
	}
	if len(passwordFields) != 0 {
		t.Errorf("expected no password verification when VerifyPasswordFields is nil, got %v", passwordFields)
	}
}

func TestGet_VerifyPasswordFields(t *testing.T) {
	recreatePasswordStructTable()

	obj := &PasswordStruct{Name: "erin", Password: "correct-horse"}
	if err := testCRUD.Save(context.Background(), obj, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	byName := func(name string) *sqlfilters.Filters {
		return &sqlfilters.Filters{
			"Name": {Op: sqlfilters.OpEqual, Val: name},
		}
	}

	// exactly one match, correct password → PassOK, non-password key ignored
	results, passwordFields, err := testCRUD.Get(context.Background(), func() interface{} {
		return &PasswordStruct{}
	}, GetOptions{
		Filters: byName("erin"),
		VerifyPasswordFields: map[string]string{
			"Password": "correct-horse",
			"Name":     "erin",
		},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if len(results) != 1 {
		t.Fatalf("Get: expected 1 result, got %d", len(results))
	}
	if got := passwordFields["Password"]; got != PassOK {
		t.Errorf("expected PassOK for correct password, got %d", got)
	}
	if _, ok := passwordFields["Name"]; ok {
		t.Errorf("expected no entry for non-password field, got %d", passwordFields["Name"])
	}
	if len(passwordFields) != 1 {
		t.Errorf("expected exactly 1 entry in passwordFields, got %d", len(passwordFields))
	}
	if ps := results[0].(*PasswordStruct); ps.Password != "" {
		t.Errorf("Get: expected password field to be empty, got %q", ps.Password)
	}

	// exactly one match, wrong password → PassInvalid
	_, passwordFields2, err := testCRUD.Get(context.Background(), func() interface{} {
		return &PasswordStruct{}
	}, GetOptions{
		Filters: byName("erin"),
		VerifyPasswordFields: map[string]string{
			"Password": "wrong-password",
		},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if got := passwordFields2["Password"]; got != PassInvalid {
		t.Errorf("expected PassInvalid for wrong password, got %d", got)
	}

	// a second record makes the match count 2, so no verification happens
	obj2 := &PasswordStruct{Name: "frank", Password: "correct-horse"}
	if err := testCRUD.Save(context.Background(), obj2, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	multiResults, passwordFields3, err := testCRUD.Get(context.Background(), func() interface{} {
		return &PasswordStruct{}
	}, GetOptions{
		VerifyPasswordFields: map[string]string{
			"Password": "correct-horse",
		},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if len(multiResults) != 2 {
		t.Fatalf("Get: expected 2 results, got %d", len(multiResults))
	}
	if len(passwordFields3) != 0 {
		t.Errorf("expected no password verification for a multi-row result, got %v", passwordFields3)
	}

	// no match at all → no verification either
	_, passwordFields4, err := testCRUD.Get(context.Background(), func() interface{} {
		return &PasswordStruct{}
	}, GetOptions{
		Filters: byName("nobody"),
		VerifyPasswordFields: map[string]string{
			"Password": "correct-horse",
		},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if len(passwordFields4) != 0 {
		t.Errorf("expected no password verification for a zero-row result, got %v", passwordFields4)
	}
}
