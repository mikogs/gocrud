package gocrud

import (
	"context"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/test"
)

// TestUpdateMultiple tests if UpdateMultiple update objects from database based on specified filters
func TestUpdateMultiple(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should not be removed
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Insert data that should be updated
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		ts.PrimaryEmail = "changeme@example.com"
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Update multiple rows from the database
	err := testCRUD.UpdateMultiple(context.Background(), &test.TestStruct{}, map[string]interface{}{
		"PrimaryEmail": "newemail@example.com",
		"Age":          98,
	},
		UpdateMultipleOptions{
			Filters: &sqlfilters.Filters{
				"Price": {
					Op:  sqlfilters.OpEqual,
					Val: 444,
				},
				"PrimaryEmail": {
					Op:  sqlfilters.OpEqual,
					Val: "changeme@example.com",
				},
			},
		})
	if err != nil {
		t.Fatalf("UpdateMultiple failed to update objects: %s %s", err.(*CRUDError).Op, err.(*CRUDError).Err.Error())
	}

	cnt, _ := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{
		Filters: &sqlfilters.Filters{
			"PrimaryEmail": {
				Op:  sqlfilters.OpEqual,
				Val: "newemail@example.com",
			},
			"Age": {
				Op:  sqlfilters.OpEqual,
				Val: 98,
			},
		},
	})
	if cnt != 150 {
		t.Fatalf("UpdateMultiple updated invalid number of rows, there are %d rows left, instead of %d", cnt, 150)
	}
}

func TestUpdateMultiple_InvalidValues(t *testing.T) {
	recreateTestStructTable()

	for i := 1; i < 11; i++ {
		_ = testCRUD.Save(context.Background(), test.TestStructWithData(), SaveOptions{})
	}

	err := testCRUD.UpdateMultiple(context.Background(), &test.TestStruct{}, map[string]interface{}{
		"FirstName": "x",
		"Age":       "notanint",
	}, UpdateMultipleOptions{})
	fatalIfNotViolations(t, err, map[string]string{
		"FirstName": "min",
		"Age":       ViolationType,
	})

	cnt, _ := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{
		Filters: &sqlfilters.Filters{
			"FirstName": {Op: sqlfilters.OpEqual, Val: "John"},
			"Age":       {Op: sqlfilters.OpEqual, Val: 37},
		},
	})
	if cnt != 10 {
		t.Fatalf("UpdateMultiple updated rows with invalid values, got %d unchanged rows, want 10", cnt)
	}
}
