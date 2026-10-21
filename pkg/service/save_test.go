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
	now := time.Now().UTC().Unix()
	err := testService.Save(context.Background(), objSaved, now, 311)
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
	objSaved.CreatedBy = 7
	objSaved.Key = "123456789012345678901234567890aaa"

	err = testService.Save(context.Background(), objSaved, now, 311)
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
	err := testService.Save(context.Background(), obj, time.Now().UTC().Unix(), 311)

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
