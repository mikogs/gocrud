package gocrud

import (
	"context"
	"errors"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/structinfo"
)

type StructWithoutID struct {
	Name string
}

type StructWithSkippedFields struct {
	ID       uint64
	Name     string
	Skipped  string `crud:"-"`
	Price    int64
	internal string
}

func TestOperations_InvalidStruct(t *testing.T) {
	ctx := context.Background()
	var nilPointer *StructWithoutID

	objs := map[string]struct {
		obj  interface{}
		want error
	}{
		"nil":          {nil, structinfo.NotStructPointerError},
		"nil pointer":  {nilPointer, structinfo.NotStructPointerError},
		"struct value": {StructWithoutID{}, structinfo.NotStructPointerError},
		"without ID":   {&StructWithoutID{}, structinfo.IDFieldError},
	}

	for name, test := range objs {
		t.Run(name, func(t *testing.T) {
			_, _, getErr := testCRUDSQLite.Get(ctx, func() interface{} { return test.obj }, GetOptions{})
			_, getCountErr := testCRUDSQLite.GetCount(ctx, test.obj, GetCountOptions{})

			errs := map[string]error{
				"CreateTable":    testCRUDSQLite.CreateTable(ctx, test.obj),
				"DropTable":      testCRUDSQLite.DropTable(ctx, test.obj),
				"Save":           testCRUDSQLite.Save(ctx, test.obj, SaveOptions{}),
				"Load":           testCRUDSQLite.Load(ctx, test.obj, "1", LoadOptions{}).Error,
				"Delete":         testCRUDSQLite.Delete(ctx, test.obj, DeleteOptions{}),
				"DeleteMultiple": testCRUDSQLite.DeleteMultiple(ctx, test.obj, DeleteMultipleOptions{}),
				"UpdateMultiple": testCRUDSQLite.UpdateMultiple(ctx, test.obj, map[string]interface{}{"Name": "x"}, UpdateMultipleOptions{}),
				"Get":            getErr,
				"GetCount":       getCountErr,
			}

			for op, err := range errs {
				if !errors.Is(err, test.want) {
					t.Fatalf("%s returned err %v, want %v", op, err, test.want)
				}
			}
		})
	}
}

func testSkippedFields(t *testing.T, crud *CRUD) {
	ctx := context.Background()

	_ = crud.DropTable(ctx, &StructWithSkippedFields{})
	if err := crud.CreateTable(ctx, &StructWithSkippedFields{}); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	obj := &StructWithSkippedFields{Name: "Apple", Skipped: "x", Price: 10, internal: "y"}
	if err := crud.Save(ctx, obj, SaveOptions{}); err != nil {
		t.Fatalf("Save failed to insert: %s", err.Error())
	}

	obj.Price = 20
	if err := crud.Save(ctx, obj, SaveOptions{}); err != nil {
		t.Fatalf("Save failed to upsert: %s", err.Error())
	}
	if err := crud.Save(ctx, obj, SaveOptions{NoInsert: true}); err != nil {
		t.Fatalf("Save failed to update: %s", err.Error())
	}

	loaded := &StructWithSkippedFields{}
	if err := crud.Load(ctx, loaded, "1", LoadOptions{}).Error; err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	want := StructWithSkippedFields{ID: 1, Name: "Apple", Price: 20}
	if *loaded != want {
		t.Fatalf("Load returned %+v, want %+v", *loaded, want)
	}

	objs, _, err := crud.Get(ctx, func() interface{} { return &StructWithSkippedFields{} }, GetOptions{
		Filters: &sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "Apple"}},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if len(objs) != 1 || *objs[0].(*StructWithSkippedFields) != want {
		t.Fatalf("Get returned %+v, want [%+v]", objs, want)
	}

	if err := crud.Delete(ctx, loaded, DeleteOptions{}); err != nil {
		t.Fatalf("Delete failed: %s", err.Error())
	}
}

func TestSkippedFields(t *testing.T) {
	testSkippedFields(t, testCRUD)
}

func TestSQLite_SkippedFields(t *testing.T) {
	testSkippedFields(t, testCRUDSQLite)
}
