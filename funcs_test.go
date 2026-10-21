package gocrud

import (
	"reflect"
	"testing"

	"github.com/mikogs/gocrud/pkg/test"
)

func TestObjSetIDValue(t *testing.T) {
	obj := &test.TestStruct{}
	ObjSetIDValue(obj, 42)
	if obj.ID != 42 || ObjIDValue(obj) != 42 {
		t.Fatalf("ObjSetIDValue set ID %d, want 42", obj.ID)
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
