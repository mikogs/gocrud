package gocrud

import (
	"context"
	"fmt"
	"testing"

	"github.com/mikogs/gocrud/pkg/test"
)

// TestLoad tests if Load properly gets row from the database table and populate object fields with its value
func TestLoad(t *testing.T) {
	recreateTestStructTable()

	// Insert an object first
	objSaved := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), objSaved, SaveOptions{})

	// Get the object
	objLoaded := &test.TestStruct{}
	output := testCRUD.Load(context.Background(), objLoaded, fmt.Sprintf("%d", objSaved.ID), LoadOptions{})
	if output.Error != nil {
		t.Fatalf("Load failed to get data: %s", output.Error.(*CRUDError).Op)
	}

	if !test.AreTestStructObjectsSame(objSaved, objLoaded) {
		t.Fatalf("Load failed to set struct with data: %s", output.Error.(*CRUDError).Op)
	}
}
