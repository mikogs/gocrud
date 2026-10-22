package gocrud

import (
	"context"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/test"
)

func TestGetCount_WithFilters(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should be ignored by GetCount later on
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Insert data that should be selected by filters
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Get the data from the database
	cnt, err := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{
		Filters: &sqlfilters.Filters{
			"Price":        {Op: sqlfilters.OpEqual, Val: 444},
			"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "primary@example.com"},
		},
	})
	if err != nil {
		t.Fatalf("Get failed to return list of objects: %s", err.(*CRUDError).Op)
	}
	if cnt != 150 {
		t.Fatalf("Get failed to return list of objects, want %v, got %v", 150, cnt)
	}
}

func TestGetCount_WithRawQuery(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should be ignored by GetCount later on
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Insert data that should be selected by filters
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Get the data from the database
	cnt, err := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{
		Filters: sqlfilters.Or(sqlfilters.Filters{
			"Price":        {Op: sqlfilters.OpEqual, Val: 444},
			"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "primary@example.com"},
		}, sqlfilters.Raw(".PrimaryEmail = ? AND .Age IN (?)", "another@example.com", []int{32, 33, 34})),
	})
	if err != nil {
		t.Fatalf("Get failed to return list of objects: %s", err.(*CRUDError).Op)
	}
	if cnt != 153 {
		t.Fatalf("Get failed to return list of objects, want %v, got %v", 153, cnt)
	}
}

func TestGetCount_InvalidFilterValue(t *testing.T) {
	_, err := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{
		Filters: &sqlfilters.Filters{
			"Age": {Op: sqlfilters.OpEqual, Val: 500},
		},
	})
	fatalIfNotViolations(t, err, map[string]string{"Age": "max"})
}

func TestGetCount_UnconvertibleStringFilter(t *testing.T) {
	_, err := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{
		Filters: &sqlfilters.Filters{
			"Age": {Op: sqlfilters.OpEqual, Val: "notanint"},
		},
		ConvertFiltersFromString: true,
	})
	fatalIfNotViolations(t, err, map[string]string{"Age": ViolationType})
}
