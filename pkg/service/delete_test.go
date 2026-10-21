package service

import (
	"context"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/test"
)

func TestDelete(t *testing.T) {
	recreateTestStructTable()

	// Insert an object first
	objSaved := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), objSaved, gocrud.SaveOptions{})

	// Delete it
	err := testService.Delete(context.Background(), "teststruct", objSaved.ID)
	if err != nil {
		t.Fatalf("Delete failed to remove: %s", err.Error())
	}

	test.FatalIfTestStructNotDeletedInTheDatabase(t, testDB, objSaved.ID)
}
