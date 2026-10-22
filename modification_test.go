package gocrud

import (
	"context"
	"testing"
	"time"

	"github.com/mikogs/gocrud/pkg/test"
)

func loadTestStruct(t *testing.T, crud *CRUD, id uint64) *test.TestStruct {
	t.Helper()

	loaded := &test.TestStruct{}
	if _, err := crud.Load(context.Background(), loaded, id, LoadOptions{}); err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if loaded.ID != id {
		t.Fatalf("Load did not find object %d", id)
	}

	return loaded
}

func fatalIfModification(t *testing.T, obj *test.TestStruct, createdAt time.Time, createdBy uint64, modifiedAt time.Time, modifiedBy uint64) {
	t.Helper()

	if !obj.CreatedAt.Equal(createdAt) || obj.CreatedBy != createdBy || !obj.ModifiedAt.Equal(modifiedAt) || obj.ModifiedBy != modifiedBy {
		t.Fatalf("got created %v by %d, modified %v by %d, want created %v by %d, modified %v by %d",
			obj.CreatedAt, obj.CreatedBy, obj.ModifiedAt, obj.ModifiedBy, createdAt, createdBy, modifiedAt, modifiedBy)
	}
}

func testModificationFields(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = crud.DropTable(ctx, &test.TestStruct{})
	_ = crud.CreateTable(ctx, &test.TestStruct{})

	forgedAt := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	// insert with the current time, ignoring the creation fields set on the object
	before := time.Now().Truncate(time.Microsecond)
	obj := test.TestStructWithData()
	obj.CreatedAt = forgedAt
	obj.CreatedBy = 99
	if err := crud.Save(ctx, obj, SaveOptions{ModifiedBy: 4}); err != nil {
		t.Fatalf("Save failed to insert: %s", err.Error())
	}
	createdAt := obj.CreatedAt
	if createdAt.Before(before) || createdAt.After(time.Now()) || createdAt.Location() != time.UTC {
		t.Fatalf("Save set CreatedAt to %v, want the current time in UTC", createdAt)
	}
	fatalIfModification(t, obj, createdAt, 4, createdAt, 4)
	fatalIfModification(t, loadTestStruct(t, crud, obj.ID), createdAt, 4, createdAt, 4)

	// update with an object that does not have the creation fields, e.g. one decoded from an HTTP request
	modifiedAt := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)
	fromRequest := test.TestStructWithData()
	fromRequest.ID = obj.ID
	if err := crud.Save(ctx, fromRequest, SaveOptions{ModifiedBy: 5, ModifiedAt: modifiedAt}); err != nil {
		t.Fatalf("Save failed to update: %s", err.Error())
	}
	fatalIfModification(t, loadTestStruct(t, crud, obj.ID), createdAt, 4, modifiedAt, 5)

	// update only, with forged creation fields
	forged := loadTestStruct(t, crud, obj.ID)
	forged.CreatedAt = forgedAt
	forged.CreatedBy = 99
	if err := crud.Save(ctx, forged, SaveOptions{NoInsert: true, ModifiedBy: 6, ModifiedAt: modifiedAt.Add(time.Hour)}); err != nil {
		t.Fatalf("Save failed to update only: %s", err.Error())
	}
	fatalIfModification(t, loadTestStruct(t, crud, obj.ID), createdAt, 4, modifiedAt.Add(time.Hour), 6)

	// "upsert" of an object with an ID that does not exist yet inserts it with the creation fields
	newWithID := test.TestStructWithData()
	newWithID.ID = 1000
	if err := crud.Save(ctx, newWithID, SaveOptions{ModifiedBy: 7, ModifiedAt: modifiedAt}); err != nil {
		t.Fatalf("Save failed to insert with an ID: %s", err.Error())
	}
	fatalIfModification(t, loadTestStruct(t, crud, 1000), modifiedAt, 7, modifiedAt, 7)

	// without a user, e.g. in a background job
	if err := crud.Save(ctx, newWithID, SaveOptions{}); err != nil {
		t.Fatalf("Save failed without a user: %s", err.Error())
	}
	loaded := loadTestStruct(t, crud, 1000)
	if loaded.ModifiedAt.Before(before) || loaded.ModifiedAt.After(time.Now()) {
		t.Fatalf("Save without a user set ModifiedAt to %v, want the current time", loaded.ModifiedAt)
	}
	fatalIfModification(t, loaded, modifiedAt, 7, loaded.ModifiedAt, 0)
}

func TestSave_ModificationFields(t *testing.T)        { testModificationFields(t, testCRUD) }
func TestSQLite_Save_ModificationFields(t *testing.T) { testModificationFields(t, testCRUDSQLite) }
