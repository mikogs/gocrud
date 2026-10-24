package gocrud

import (
	"errors"
	"testing"

	"github.com/mikogs/gocrud/internal/fixtures"
)

func TestObjSetIDValue(t *testing.T) {
	for _, id := range []any{uint64(42), 42, int8(42), uint16(42), "42"} {
		obj := &fixtures.TestStruct{}
		if err := ObjSetIDValue(obj, id); err != nil || obj.ID != 42 || ObjIDValue(obj) != uint64(42) {
			t.Fatalf("ObjSetIDValue(%#v) returned %v and set ID %d, want 42", id, err, obj.ID)
		}
	}

	ticket := &Ticket{}
	if err := ObjSetIDValue(ticket, "0190F5C3-6D1E-7B3A-9C4D-2E5F6A7B8C9D"); err != nil || ticket.ID != "0190f5c3-6d1e-7b3a-9c4d-2e5f6a7b8c9d" {
		t.Fatalf("ObjSetIDValue returned %v and set ID %q, want the UUID in its standard form", err, ticket.ID)
	}

	for _, obj := range []any{&fixtures.TestStruct{ID: 42}, ticket} {
		if err := ObjSetIDValue(obj, nil); err != nil || !ObjIDIsZero(obj) {
			t.Fatalf("ObjSetIDValue(nil) returned %v and left ID %v, want it cleared", err, ObjIDValue(obj))
		}
	}

	invalid := map[string]struct {
		obj any
		id  any
	}{
		"negative number":       {&fixtures.TestStruct{}, -1},
		"text for a number":     {&fixtures.TestStruct{}, "forty-two"},
		"float":                 {&fixtures.TestStruct{}, 4.2},
		"number for a UUID":     {&Ticket{}, 42},
		"text that is not UUID": {&Ticket{}, "42"},
	}
	for name, test := range invalid {
		if err := ObjSetIDValue(test.obj, test.id); !errors.Is(err, ErrInvalidID) || !ObjIDIsZero(test.obj) {
			t.Fatalf("%s: ObjSetIDValue returned %v, want %v and the ID not set", name, err, ErrInvalidID)
		}
	}
}
