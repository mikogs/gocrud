package gocrud

import (
	"context"
	"errors"
	"testing"

	"github.com/mikogs/gocrud/pkg/gocrudtest"
)

func testSave(t *testing.T, crud *CRUD) {
	recreateTestStructs(t, crud, 0, nil)
	ctx := context.Background()

	saved := gocrudtest.TestStructWithData()
	if err := crud.Save(ctx, saved, SaveOptions{}); err != nil || saved.ID == 0 {
		t.Fatalf("Save returned %v and ID %d, want the object inserted", err, saved.ID)
	}
	if loaded := loadTestStruct(t, crud, saved.ID); !gocrudtest.AreTestStructObjectsSame(saved, loaded) {
		t.Fatalf("Save inserted %+v, want %+v", loaded, saved)
	}

	saved.PrimaryEmail = "updated@example.com"
	saved.Age = 50
	if err := crud.Save(ctx, saved, SaveOptions{}); err != nil {
		t.Fatalf("Save failed to update: %s", err.Error())
	}
	if loaded := loadTestStruct(t, crud, saved.ID); !gocrudtest.AreTestStructObjectsSame(saved, loaded) {
		t.Fatalf("Save updated %+v, want %+v", loaded, saved)
	}
}

func testSaveWithID(t *testing.T, crud *CRUD) {
	recreateTestStructs(t, crud, 0, nil)
	ctx := context.Background()

	saved := gocrudtest.TestStructWithData()
	saved.ID = 99999
	if err := crud.Save(ctx, saved, SaveOptions{}); err != nil {
		t.Fatalf("Save failed to insert an object with an ID: %s", err.Error())
	}

	saved.FirstName = "Updated"
	if err := crud.Save(ctx, saved, SaveOptions{}); err != nil {
		t.Fatalf("Save failed to update an object with an ID: %s", err.Error())
	}
	if loaded := loadTestStruct(t, crud, 99999); loaded.FirstName != "Updated" {
		t.Fatalf("Save stored %+v, want the object with ID 99999 updated", loaded)
	}

	saved.FirstName = "NoInsert"
	if err := crud.Save(ctx, saved, SaveOptions{NoInsert: true}); err != nil {
		t.Fatalf("Save with NoInsert failed to update: %s", err.Error())
	}
	if loaded := loadTestStruct(t, crud, 99999); loaded.FirstName != "NoInsert" {
		t.Fatalf("Save with NoInsert stored %+v, want the object updated", loaded)
	}
	if err := crud.Save(ctx, saved, SaveOptions{NoInsert: true}); err != nil {
		t.Fatalf("Save with NoInsert and the same values failed: %s", err.Error())
	}

	missing := gocrudtest.TestStructWithData()
	missing.ID = 88888
	if err := crud.Save(ctx, missing, SaveOptions{NoInsert: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Save with NoInsert of an object that does not exist returned %v, want %v", err, ErrNotFound)
	}
	if cnt, _ := GetCount[gocrudtest.TestStruct](ctx, crud, GetCountOptions{}); cnt != 1 {
		t.Fatalf("Save with NoInsert inserted an object, got %d rows, want 1", cnt)
	}
}

func duplicateKeyTestStruct(t *testing.T, crud *CRUD) *gocrudtest.TestStruct {
	t.Helper()

	saved := gocrudtest.TestStructWithData()
	recreateTestStructs(t, crud, 1, func(int) *gocrudtest.TestStruct { return saved })

	duplicate := gocrudtest.TestStructWithData()
	duplicate.Key = saved.Key
	return duplicate
}

func testSaveUniq(t *testing.T, crud *CRUD, wantField string) {
	err := crud.Save(context.Background(), duplicateKeyTestStruct(t, crud), SaveOptions{})
	var uniqError *UniqError
	if !errors.As(err, &uniqError) || uniqError.Field != wantField || uniqError.Error() != "uniq failed for field: "+wantField {
		t.Fatalf("Save of a duplicate key returned %v, want a UniqError for %s", err, wantField)
	}
}

func testSaveInvalid(t *testing.T, crud *CRUD) {
	recreateTestStructs(t, crud, 0, nil)

	obj, wantViolations := invalidTestStruct()
	err := crud.Save(context.Background(), obj, SaveOptions{})
	fatalIfNotViolations(t, err, wantViolations)

	if cnt, _ := GetCount[gocrudtest.TestStruct](context.Background(), crud, GetCountOptions{}); cnt != 0 {
		t.Fatalf("Save inserted an invalid object, got %d rows, want 0", cnt)
	}
}

func TestSave(t *testing.T)        { testSave(t, testCRUD) }
func TestSQLite_Save(t *testing.T) { testSave(t, testCRUDSQLite) }

func TestSave_WithID(t *testing.T)        { testSaveWithID(t, testCRUD) }
func TestSQLite_Save_WithID(t *testing.T) { testSaveWithID(t, testCRUDSQLite) }

func TestSave_Uniq(t *testing.T)        { testSaveUniq(t, testCRUD, "Key") }
func TestSQLite_Save_Uniq(t *testing.T) { testSaveUniq(t, testCRUDSQLite, "Key") }

func TestSave_Invalid(t *testing.T)        { testSaveInvalid(t, testCRUD) }
func TestSQLite_Save_Invalid(t *testing.T) { testSaveInvalid(t, testCRUDSQLite) }

func TestSave_UniqUsingGetCount(t *testing.T) {
	duplicate := duplicateKeyTestStruct(t, testCRUD)

	if _, err := testDB.Exec("ALTER TABLE test_struct DROP CONSTRAINT IF EXISTS test_struct_key_key;"); err != nil {
		t.Fatalf("Exec failed to remove the UNIQUE constraint: %s", err.Error())
	}

	// a CRUD of its own, so that the flag does not change the other tests
	uniqCRUD := New(testDB, Options{Dialect: DialectPostgres})
	uniqCRUD.SetFlag(GetCountOnUniq)

	err := uniqCRUD.Save(context.Background(), duplicate, SaveOptions{})
	var uniqError *UniqError
	if !errors.As(err, &uniqError) || uniqError.Field != "Key" {
		t.Fatalf("Save of a duplicate key without a UNIQUE constraint returned %v, want a UniqError for Key", err)
	}
}
