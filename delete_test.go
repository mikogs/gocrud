package gocrud

import (
	"context"
	"testing"

	"github.com/mikogs/gocrud/pkg/test"
)

// TestDelete tests if Delete removes an object from the database.
func TestDelete(t *testing.T) {
	recreateTestStructTable()

	// Insert an object first
	objSaved := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), objSaved, SaveOptions{})

	// Delete it
	err := testCRUD.Delete(context.Background(), objSaved, DeleteOptions{})
	if err != nil {
		t.Fatalf("Delete failed to remove: %s", err.(*CRUDError).Op)
	}

	test.FatalIfTestStructNotDeletedInTheDatabase(t, testDB, objSaved.ID)

	if objSaved.ID != 0 {
		t.Fatalf("Delete failed to set ID to 0 on the struct")
	}
}
