package gocrud

import (
	"errors"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud/pkg/test"
)

func TestObjSetIDValue(t *testing.T) {
	for _, id := range []interface{}{uint64(42), 42, int8(42), uint16(42), "42"} {
		obj := &test.TestStruct{}
		if err := ObjSetIDValue(obj, id); err != nil || obj.ID != 42 || ObjIDValue(obj) != uint64(42) {
			t.Fatalf("ObjSetIDValue(%#v) returned %v and set ID %d, want 42", id, err, obj.ID)
		}
	}

	ticket := &Ticket{}
	if err := ObjSetIDValue(ticket, "0190F5C3-6D1E-7B3A-9C4D-2E5F6A7B8C9D"); err != nil || ticket.ID != "0190f5c3-6d1e-7b3a-9c4d-2e5f6a7b8c9d" {
		t.Fatalf("ObjSetIDValue returned %v and set ID %q, want the UUID in its standard form", err, ticket.ID)
	}

	for _, obj := range []interface{}{&test.TestStruct{ID: 42}, ticket} {
		if err := ObjSetIDValue(obj, nil); err != nil || !ObjIDIsZero(obj) {
			t.Fatalf("ObjSetIDValue(nil) returned %v and left ID %v, want it cleared", err, ObjIDValue(obj))
		}
	}

	invalid := map[string]struct {
		obj interface{}
		id  interface{}
	}{
		"negative number":       {&test.TestStruct{}, -1},
		"text for a number":     {&test.TestStruct{}, "forty-two"},
		"float":                 {&test.TestStruct{}, 4.2},
		"number for a UUID":     {&Ticket{}, 42},
		"text that is not UUID": {&Ticket{}, "42"},
	}
	for name, test := range invalid {
		if err := ObjSetIDValue(test.obj, test.id); !errors.Is(err, ErrInvalidID) || !ObjIDIsZero(test.obj) {
			t.Fatalf("%s: ObjSetIDValue returned %v, want %v and the ID not set", name, err, ErrInvalidID)
		}
	}
}

func TestPasswordFieldsFor(t *testing.T) {
	if got := testCRUDSQLite.PasswordFieldsFor(&PasswordStruct{}); !reflect.DeepEqual(got, []string{"Password"}) {
		t.Fatalf("PasswordFieldsFor returned %v, want [Password]", got)
	}
	if got := testCRUDSQLite.PasswordFieldsFor(&test.TestStruct{}); len(got) != 0 {
		t.Fatalf("PasswordFieldsFor returned %v for a struct without password fields, want none", got)
	}
	if got := testCRUDSQLite.PasswordFieldsFor(nil); got != nil {
		t.Fatalf("PasswordFieldsFor returned %v for an invalid object, want nil", got)
	}
}

func TestNew_InvalidTableNamePrefix(t *testing.T) {
	for _, prefix := range []string{`app"; DROP TABLE x; --`, "App_", "app-", "app."} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("New with the table name prefix %q did not panic", prefix)
				}
			}()
			New(testDBSQLite, Options{Dialect: DialectSQLite, TableNamePrefix: prefix})
		}()
	}

	New(testDBSQLite, Options{Dialect: DialectSQLite, TableNamePrefix: "app_2_"})
}
