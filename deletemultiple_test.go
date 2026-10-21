package gocrud

import (
	"context"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/test"
)

// TestDeleteMultiple tests if DeleteMultiple removes objects from database based on specified filters
func TestDeleteMultiple_WithFilters(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should not be removed
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Insert data that should be deleted
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Delete multiple rows from the database
	err := testCRUD.DeleteMultiple(context.Background(), &test.TestStruct{}, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{
			"Price": {
				Op:  sqlfilters.OpEqual,
				Val: 444,
			},
			"PrimaryEmail": {
				Op:  sqlfilters.OpEqual,
				Val: "primary@example.com",
			},
		},
	})
	if err != nil {
		t.Fatalf("DeleteMultiple failed to delete objects: %s", err.(*CRUDError).Op)
	}

	cnt, _ := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{})
	if cnt != 50 {
		t.Fatalf("DeleteMultiple removed invalid number of rows, there are %d rows left, instead of %d", cnt, 50)
	}
}

// TestDeleteMultipleWithRawQuery tests if DeleteMultiple removes objects from the database based on specified filters
// and condition, that is almost a raw query.
func TestDeleteMultiple_WithRawQuery(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should not be removed
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Insert data that should be deleted
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Delete multiple rows from the database
	err := testCRUD.DeleteMultiple(context.Background(), &test.TestStruct{}, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{
			"Price": {
				Op:  sqlfilters.OpEqual,
				Val: 444,
			},
			"PrimaryEmail": {
				Op:  sqlfilters.OpEqual,
				Val: "primary@example.com",
			},
			sqlfilters.Raw: {
				Op: sqlfilters.OpOR,
				Val: []interface{}{
					".Age = ? OR .Age IN (?) OR (.Age = ? AND .PrimaryEmail = ?)",
					31,
					[]int{32, 33, 34},
					35,
					"miko@example.com",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("DeleteMultiple failed to delete objects: %s", err.(*CRUDError).Op)
	}

	cnt, _ := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{})
	if cnt != 46 {
		t.Fatalf("DeleteMultiple removed invalid number of rows, there are %d rows left, instead of %d", cnt, 46)
	}
}

// TestDeleteMultipleWithRawQueryOnly tests if DeleteMultiple removes objects from database based on a condition which is somewhat raw query
func TestDeleteMultiple_WithRawQueryOnly(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should not be removed
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Insert data that should be deleted
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Delete multiple rows from the database
	err := testCRUD.DeleteMultiple(context.Background(), &test.TestStruct{}, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{
			sqlfilters.Raw: {
				Op: sqlfilters.OpOR,
				Val: []interface{}{
					"(.Price = ? AND .PrimaryEmail = ?) OR (.Age = ? OR .Age IN (?) OR (.Age = ? AND .PrimaryEmail = ?))",
					444,
					"primary@example.com",
					31,
					[]int{32, 33, 34},
					35,
					"miko@example.com",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("DeleteMultiple failed to delete objects: %s", err.(*CRUDError).Op)
	}

	cnt, _ := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{})
	if cnt != 46 {
		t.Fatalf("DeleteMultiple removed invalid number of rows, there are %d rows left, instead of %d", cnt, 46)
	}
}
