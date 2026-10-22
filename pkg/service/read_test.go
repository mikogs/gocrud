package service

import (
	"context"
	"errors"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/test"
)

func TestRead(t *testing.T) {
	recreateTestStructTable()
	ctx := context.Background()

	saved := test.TestStructWithData()
	if err := testService.Save(ctx, saved, uint64(1)); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	for _, id := range []interface{}{saved.ID, int(saved.ID), "1"} {
		obj, _, err := testService.Read(ctx, "teststruct", id, nil, nil)
		if err != nil || !test.AreTestStructObjectsSame(saved, obj.(*test.TestStruct)) {
			t.Fatalf("Read(%#v) returned %+v, %v, want the object", id, obj, err)
		}
	}

	if _, _, err := testService.Read(ctx, "teststruct", 444, nil, nil); !errors.Is(err, ErrNotFound) || !errors.Is(err, gocrud.ErrNotFound) {
		t.Fatalf("Read of an ID that does not exist returned %v, want %v", err, ErrNotFound)
	}
	if _, _, err := testService.Read(ctx, "teststruct", "one", nil, nil); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Read of a text ID returned %v, want %v", err, ErrInvalidID)
	}
}
