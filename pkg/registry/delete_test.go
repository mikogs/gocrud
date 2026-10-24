package registry

import (
	"context"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/internal/fixtures"
)

func TestDelete(t *testing.T) {
	recreateTestStructTable()

	objSaved := fixtures.TestStructWithData()
	_ = testCRUD.Save(context.Background(), objSaved, gocrud.SaveOptions{})

	err := testService.Delete(context.Background(), "teststruct", objSaved.ID)
	if err != nil {
		t.Fatalf("Delete failed to remove: %s", err.Error())
	}

	fixtures.FatalIfTestStructNotDeletedInTheDatabase(t, testDB, objSaved.ID)
}
