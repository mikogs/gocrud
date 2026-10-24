package registry

import (
	"context"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/gocrudtest"
)

func TestDelete(t *testing.T) {
	recreateTestStructTable()

	objSaved := gocrudtest.TestStructWithData()
	_ = testCRUD.Save(context.Background(), objSaved, gocrud.SaveOptions{})

	err := testService.Delete(context.Background(), "teststruct", objSaved.ID)
	if err != nil {
		t.Fatalf("Delete failed to remove: %s", err.Error())
	}

	gocrudtest.FatalIfTestStructNotDeletedInTheDatabase(t, testDB, objSaved.ID)
}
