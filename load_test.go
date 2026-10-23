package gocrud

import (
	"context"
	"errors"
	"testing"

	"github.com/mikogs/gocrud/pkg/gocrudtest"
)

func testLoad(t *testing.T, crud *CRUD) {
	recreateTestStructs(t, crud, 1, func(int) *gocrudtest.TestStruct { return gocrudtest.TestStructWithData() })
	ctx := context.Background()

	loaded := &gocrudtest.TestStruct{}
	if _, err := crud.Load(ctx, loaded, "1", LoadOptions{}); err != nil || loaded.ID != 1 || loaded.FirstName != "John" {
		t.Fatalf("Load returned %v and %+v, want the object", err, loaded)
	}

	if _, err := crud.Load(ctx, loaded, 2, LoadOptions{}); err != nil || !ObjIDIsZero(loaded) || loaded.FirstName != "" {
		t.Fatalf("Load of an ID that does not exist returned %v and %+v, want the fields zeroed", err, loaded)
	}

	if _, err := crud.Load(ctx, loaded, "one", LoadOptions{}); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Load of an invalid ID returned %v, want %v", err, ErrInvalidID)
	}
}

func testDelete(t *testing.T, crud *CRUD) {
	recreateTestStructs(t, crud, 2, func(int) *gocrudtest.TestStruct { return gocrudtest.TestStructWithData() })
	ctx := context.Background()

	obj := loadTestStruct(t, crud, 1)
	if err := crud.Delete(ctx, obj, DeleteOptions{}); err != nil {
		t.Fatalf("Delete failed: %s", err.Error())
	}
	if obj.ID != 0 || obj.FirstName != "" {
		t.Fatalf("Delete left the fields %+v, want them zeroed", obj)
	}

	if _, err := Load[gocrudtest.TestStruct](ctx, crud, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load of the deleted object returned %v, want %v", err, ErrNotFound)
	}
	if cnt, _ := GetCount[gocrudtest.TestStruct](ctx, crud, GetCountOptions{}); cnt != 1 {
		t.Fatalf("Delete left %d objects, want 1", cnt)
	}
}

func TestLoad(t *testing.T)        { testLoad(t, testCRUD) }
func TestSQLite_Load(t *testing.T) { testLoad(t, testCRUDSQLite) }

func TestDelete(t *testing.T)        { testDelete(t, testCRUD) }
func TestSQLite_Delete(t *testing.T) { testDelete(t, testCRUDSQLite) }
