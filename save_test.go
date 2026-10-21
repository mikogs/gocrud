package gocrud

import (
	"context"
	"errors"
	"testing"

	"github.com/mikogs/gocrud/pkg/test"
)

// TestSave tests if Save properly inserts and updates an object in the database
func TestSave(t *testing.T) {
	recreateTestStructTable()

	objSaved := test.TestStructWithData()
	err := testCRUD.Save(context.Background(), objSaved, SaveOptions{})
	if err != nil {
		t.Fatalf("Save failed to insert struct to the table: %s", err.(*CRUDError).Op)
	}

	objLoaded := &test.TestStruct{}
	fieldsPtrs := []interface{}{&objLoaded.ID}
	fieldsPtrs = append(fieldsPtrs, ObjFieldInterfaces(objLoaded, false)...)
	err = testDB.QueryRow("SELECT * FROM test_struct ORDER BY id DESC LIMIT 1").Scan(fieldsPtrs...)
	if err != nil {
		t.Fatalf("Save failed to insert struct to the table: %s", err.Error())
	}

	if objLoaded.ID == 0 || objLoaded.Flags != objSaved.Flags || objLoaded.PrimaryEmail != objSaved.PrimaryEmail ||
		objLoaded.EmailSecondary != objSaved.EmailSecondary || objLoaded.FirstName != objSaved.FirstName ||
		objLoaded.LastName != objSaved.LastName || objLoaded.Age != objSaved.Age || objLoaded.Price != objSaved.Price ||
		objLoaded.PostCode != objSaved.PostCode || objLoaded.PostCode2 != objSaved.PostCode2 ||
		objLoaded.CreatedBy != objSaved.CreatedBy || objLoaded.Key != objSaved.Key || objLoaded.Password != objSaved.Password {
		t.Fatalf("Save failed to insert struct to the table")
	}

	// Now, update the object in the database
	objSaved.Flags = 7
	objSaved.PrimaryEmail = "primary1@example.com"
	objSaved.EmailSecondary = "secondary2@example.com"
	objSaved.FirstName = "Johnny"
	objSaved.LastName = "Smithsy"
	objSaved.Age = 50
	objSaved.Price = 222
	objSaved.PostCode = "22-222"
	objSaved.PostCode2 = "33-333"
	objSaved.Password = "xxx"
	objSaved.CreatedBy = 7
	objSaved.Key = "123456789012345678901234567890aaa"

	err = testCRUD.Save(context.Background(), objSaved, SaveOptions{})
	if err != nil {
		t.Fatalf("Save failed to update struct")
	}

	objLoaded = &test.TestStruct{}
	fieldsPtrs = []interface{}{&objLoaded.ID}
	fieldsPtrs = append(fieldsPtrs, ObjFieldInterfaces(objLoaded, false)...)
	err = testDB.QueryRow("SELECT * FROM test_struct ORDER BY id DESC LIMIT 1").Scan(fieldsPtrs...)
	if err != nil {
		t.Fatalf("Save failed to update struct in the table: %s", err.Error())
	}

	if objLoaded.ID == 0 {
		t.Fatalf("Save failed to update struct in the table")
	}
}

// TestSaveInsertWithID tests if an element with provided ID will be inserted
func TestSave_InsertWithID(t *testing.T) {
	recreateTestStructTable()

	objSaved := test.TestStructWithData()
	objSaved.ID = 99999
	objSaved.FirstName = "ProvidedID"
	err := testCRUD.Save(context.Background(), objSaved, SaveOptions{})
	if err != nil {
		t.Fatalf("Save failed to insert struct with provided ID to the table: %s", err.(*CRUDError).Op)
	}

	objLoaded := &test.TestStruct{}
	fieldsPtrs := ObjFieldInterfaces(objLoaded, true)
	err = testDB.QueryRow("SELECT * FROM test_struct WHERE id=99999 ORDER BY id DESC LIMIT 1").Scan(fieldsPtrs...)
	if err != nil {
		t.Fatalf("Save failed to insert struct with provided ID in the table: %s", err.Error())
	}

	if objLoaded.ID != 99999 || objLoaded.FirstName != objSaved.FirstName {
		t.Fatalf("Save failed to insert struct with provided ID in the table")
	}

	// update that object
	objLoaded.FirstName = "UpdatedProvidedID"
	err = testCRUD.Save(context.Background(), objLoaded, SaveOptions{})
	if err != nil {
		t.Fatalf("Save failed to update struct previously inserted with provided ID to the table: %s", err.(*CRUDError).Op)
	}

	objLoadedAgain := &test.TestStruct{}
	fieldsPtrs = ObjFieldInterfaces(objLoadedAgain, true)
	err = testDB.QueryRow("SELECT * FROM test_struct WHERE id=99999 ORDER BY id DESC LIMIT 1").Scan(fieldsPtrs...)
	if err != nil {
		t.Fatalf("Save failed to update struct previously inserted with provided ID in the table: %s", err.Error())
	}
	if objLoadedAgain.ID != 99999 || objLoadedAgain.FirstName != objLoaded.FirstName {
		t.Fatalf("Save failed to update struct previously inserted with provided ID in the table")
	}
}

// TestSaveInsertWithIDAndNoInsert tests if an element with provided ID is not inserted when NoInsert is true
func TestSave_InsertWithIDAndNoInsert(t *testing.T) {
	recreateTestStructTable()

	objSaved := test.TestStructWithData()
	objSaved.ID = 99999
	objSaved.FirstName = "ProvidedID"
	err := testCRUD.Save(context.Background(), objSaved, SaveOptions{
		NoInsert: true,
	})
	if err != nil {
		t.Fatalf("Save failed to not insert struct with provided ID to the table when NoInsert: %s", err.(*CRUDError).Op)
	}

	var rowCount int
	err = testDB.QueryRow("SELECT COUNT(*) FROM test_struct WHERE id=99999").Scan(&rowCount)
	if err != nil {
		t.Fatalf("Save failed to not insert struct with provided ID in the table when NoInsert: %s", err.Error())
	}

	if rowCount > 0 {
		t.Fatalf("Save failed to not insert struct with provided ID in the table when NoInsert")
	}
}

func TestSave_ErrUniq(t *testing.T) {
	recreateTestStructTable()

	objSaved := test.TestStructWithData()
	objSaved.Key = "gottabeunique123456789012345678901234567890"
	_ = testCRUD.Save(context.Background(), objSaved, SaveOptions{})

	objWithSameKey := test.TestStructWithData()
	objWithSameKey.Key = "gottabeunique123456789012345678901234567890"
	err := testCRUD.Save(context.Background(), objWithSameKey, SaveOptions{})
	var uniqError *UniqError
	if !errors.As(err, &uniqError) || uniqError.Field != "test_struct_key_key" || uniqError.Error() != "uniq failed for field: test_struct_key_key" {
		t.Fatalf("Save failed to return uniq error for test_struct_key_key, got: %v", err)
	}
}

func TestSave_ErrUniqUsingGetCount(t *testing.T) {
	recreateTestStructTable()

	objSaved := test.TestStructWithData()
	objSaved.Key = "gottabeunique123456789012345678901234567890"
	_ = testCRUD.Save(context.Background(), objSaved, SaveOptions{})

	// a CRUD of its own, so that the flag does not change the other tests
	uniqCRUD := New(testDB, Options{Dialect: DialectPostgres})
	uniqCRUD.SetFlag(GetCountOnUniq)

	_, err := testDB.Exec("ALTER TABLE test_struct DROP CONSTRAINT IF EXISTS test_struct_key_key;")
	if err != nil {
		t.Fatalf("Exec failed to remove UNIQUE constraint: %s", err.Error())
	}

	objWithSameKey := test.TestStructWithData()
	objWithSameKey.Key = "gottabeunique123456789012345678901234567890"
	err = uniqCRUD.Save(context.Background(), objWithSameKey, SaveOptions{})
	var uniqError *UniqError
	if !errors.As(err, &uniqError) || uniqError.Field != "Key" {
		t.Fatalf("Save failed to return uniq error for Key when there is no UNIQUE constraint, got: %v", err)
	}
}

func TestSave_InvalidObject(t *testing.T) {
	recreateTestStructTable()

	obj, wantViolations := invalidTestStruct()
	err := testCRUD.Save(context.Background(), obj, SaveOptions{})
	fatalIfNotViolations(t, err, wantViolations)

	cnt, _ := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{})
	if cnt != 0 {
		t.Fatalf("Save inserted an invalid object, got %d rows, want 0", cnt)
	}
}
