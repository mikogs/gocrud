package registry

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/gocrudtest"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

func TestCRUD_Accessors(t *testing.T) {
	obj := testService.New("teststruct")
	if _, ok := obj.(*gocrudtest.TestStruct); !ok {
		t.Fatalf("New returned %T, want *gocrudtest.TestStruct", obj)
	}
	if testService.New("unknown") != nil {
		t.Fatal("New returned an object for an unknown key")
	}

	if id := testService.ID(&gocrudtest.TestStruct{ID: 42}); id != uint64(42) {
		t.Fatalf("ID returned %#v, want uint64(42)", id)
	}

	if got := testService.PasswordFieldNames("passwordstruct"); !reflect.DeepEqual(got, []string{"Password"}) {
		t.Fatalf("PasswordFieldNames returned %v, want [Password]", got)
	}
	if got := testService.PasswordFieldNames("teststruct"); len(got) != 0 {
		t.Fatalf("PasswordFieldNames returned %v for a struct without password fields, want none", got)
	}
	if got := testService.PasswordFieldNames("unknown"); got != nil {
		t.Fatalf("PasswordFieldNames returned %v for an unknown key, want nil", got)
	}

	keys := testService.Registry()
	sort.Strings(keys)
	if want := []string{"passwordstruct", "passwordstructtouch", "teststruct"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("Registry returned %v, want %v", keys, want)
	}
}

func TestOp(t *testing.T) {
	tests := map[string]int{
		"eq":    sqlfilters.OpEqual,
		"ne":    sqlfilters.OpNotEqual,
		"lt":    sqlfilters.OpLower,
		"le":    sqlfilters.OpLowerOrEqual,
		"gt":    sqlfilters.OpGreater,
		"ge":    sqlfilters.OpGreaterOrEqual,
		"like":  sqlfilters.OpLike,
		"match": sqlfilters.OpMatch,
		"bit":   sqlfilters.OpBit,
		"in":    sqlfilters.OpIn,
	}
	for op, want := range tests {
		if got, err := Op(op); err != nil || got != want {
			t.Fatalf("Op(%q) returned %d, %v, want %d", op, got, err, want)
		}
	}

	for _, op := range []string{"", "EQ", "equals", "nin"} {
		if _, err := Op(op); !errors.Is(err, ErrInvalidFilterOp) {
			t.Fatalf("Op(%q) returned err %v, want %v", op, err, ErrInvalidFilterOp)
		}
	}
}

func TestValidationErrors_Messages(t *testing.T) {
	cause := errors.New("validation failed")
	for _, err := range []error{
		&ModelValidationError{Err: cause, Violations: map[string]string{"Name": "required"}},
		&FilterValidationError{Err: cause, Violations: map[string]string{"Age": "max"}},
	} {
		if err.Error() != "validation failed" {
			t.Fatalf("%T returned message %q, want the message of the cause", err, err.Error())
		}
	}

	if !errors.Is(ErrNotFound, gocrud.ErrNotFound) {
		t.Fatal("ErrNotFound is not gocrud.ErrNotFound")
	}
}
