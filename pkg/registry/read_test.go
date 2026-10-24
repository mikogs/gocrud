package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/internal/fixtures"
)

func TestRead(t *testing.T) {
	recreateTestStructTable()
	ctx := context.Background()

	saved := fixtures.TestStructWithData()
	if err := testService.Save(ctx, saved, uint64(1)); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	for _, id := range []interface{}{saved.ID, int(saved.ID), "1"} { // #nosec G115 -- the first ID
		obj, _, err := testService.Read(ctx, "teststruct", id, nil, nil)
		if err != nil || !fixtures.AreTestStructObjectsSame(saved, obj.(*fixtures.TestStruct)) {
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
