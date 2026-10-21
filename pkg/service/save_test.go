package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mikogs/gocrud/pkg/test"
)

func TestSave(t *testing.T) {
	recreateTestStructTable()

	objSaved := test.TestStructWithData()
	err := testService.Save(context.Background(), objSaved, 311)
	if err != nil {
		t.Fatalf("Save failed to insert struct to the table: %s", err.Error())
	}

	objLoaded := test.FatalIfNoTestStructInTheDatabase(t, testDB)
	if objLoaded.ID == 0 || !test.AreTestStructObjectsSame(objSaved, objLoaded) {
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
	objSaved.Key = "123456789012345678901234567890aaa"

	err = testService.Save(context.Background(), objSaved, 311)
	if err != nil {
		t.Fatalf("Save failed to update struct")
	}

	objLoaded = test.FatalIfNoTestStructInTheDatabase(t, testDB)
	if objLoaded.ID == 0 || !test.AreTestStructObjectsSame(objSaved, objLoaded) {
		t.Fatalf("Save failed to insert struct to the table")
	}
}

func TestSave_ModelValidationError(t *testing.T) {
	recreateTestStructTable()

	obj := test.TestStructWithData()
	obj.EmailSecondary = "invalidemail"
	obj.FirstName = "x"
	err := testService.Save(context.Background(), obj, 311)

	var modelErr *ModelValidationError
	if !errors.As(err, &modelErr) {
		t.Fatalf("Save returned %v, want *ModelValidationError", err)
	}

	want := map[string]string{"EmailSecondary": "email", "FirstName": "min"}
	if !reflect.DeepEqual(modelErr.Violations, want) {
		t.Fatalf("Save returned violations %v, want %v", modelErr.Violations, want)
	}

	num, _ := testService.Num(context.Background(), "teststruct", nil, nil)
	if num != 0 {
		t.Fatalf("Save inserted an invalid object, got %d rows, want 0", num)
	}
}

func TestSave_ModificationFields(t *testing.T) {
	recreateTestStructTable()
	ctx := context.Background()
	before := time.Now().Truncate(time.Microsecond)

	obj := test.TestStructWithData()
	if err := testService.Save(ctx, obj, 311); err != nil {
		t.Fatalf("Save failed to insert: %s", err.Error())
	}
	if obj.CreatedBy != 311 || obj.ModifiedBy != 311 || obj.CreatedAt.Before(before) || !obj.ModifiedAt.Equal(obj.CreatedAt) {
		t.Fatalf("Save set created %v by %d, modified %v by %d", obj.CreatedAt, obj.CreatedBy, obj.ModifiedAt, obj.ModifiedBy)
	}

	edited := test.TestStructWithData()
	edited.ID = obj.ID
	if err := testService.Save(ctx, edited, 312); err != nil {
		t.Fatalf("Save failed to update: %s", err.Error())
	}

	loaded, _, err := testService.Read(ctx, "teststruct", obj.ID, nil, nil)
	if err != nil {
		t.Fatalf("Read failed: %s", err.Error())
	}
	got := loaded.(*test.TestStruct)
	if !got.CreatedAt.Equal(obj.CreatedAt) || got.CreatedBy != 311 || got.ModifiedBy != 312 || got.ModifiedAt.Before(obj.ModifiedAt) {
		t.Fatalf("Read returned created %v by %d, modified %v by %d, want created %v by 311, modified by 312",
			got.CreatedAt, got.CreatedBy, got.ModifiedAt, got.ModifiedBy, obj.CreatedAt)
	}
}
