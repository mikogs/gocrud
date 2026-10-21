package service

import (
	"context"
	"errors"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/test"
)

func TestRead_WhenObjectExists(t *testing.T) {
	recreateTestStructTable()

	// Insert an object first
	objSaved := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), objSaved, gocrud.SaveOptions{})

	// Get the object
	objRead, _, err := testService.Read(context.Background(), "teststruct", objSaved.ID, nil, nil)
	if err != nil {
		t.Fatalf("Read failed to get object: %s", err.Error())
	}
	if !test.AreTestStructObjectsSame(objSaved, objRead.(*test.TestStruct)) {
		t.Fatalf("Read failed to set object with data")
	}
}

func TestRead_WhenObjectDoesNotExist(t *testing.T) {
	recreateTestStructTable()

	_, _, err := testService.Read(context.Background(), "teststruct", 444, nil, nil)
	if err != nil {
		if !errors.Is(err, NotFoundError) {
			t.Fatalf("Read failed to get object: %s", err.Error())
		}
	}
	if err == nil {
		t.Fatalf("Read failed to return NotFoundError")
	}
}
