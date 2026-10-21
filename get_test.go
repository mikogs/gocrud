package gocrud

import (
	"context"
	"fmt"
	"html"
	"reflect"
	"strings"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/test"
)

// TestGet tests if Get properly gets many objects from the database, filtered and ordered, with results limited to specific number
func TestGet_WithFilters(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should be ignored by Get later on
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Insert data that should be selected by filters
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Get the data from the database
	testStructs, _, err := testCRUD.Get(context.Background(), func() interface{} {
		return &test.TestStruct{}
	}, GetOptions{
		Order:  []string{"Age", "asc", "Price", "asc"},
		Limit:  10,
		Offset: 20,
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
				Op: sqlfilters.OpAND,
				Val: []interface{}{
					".Price > ? AND .Price NOT IN (?)",
					-200,
					[]int{9999, 9998, 9997},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Get failed to return list of objects: %s", err.(*CRUDError).Op)
	}
	if len(testStructs) != 10 {
		t.Fatalf("Get failed to return list of objects, want %v, got %v", 10, len(testStructs))
	}
	if testStructs[2].(*test.TestStruct).Age != 53 {
		t.Fatalf("Get failed to return correct list of objects, want %v, got %v", 53, testStructs[2].(*test.TestStruct).Age)
	}
}

// TestGetWithStringFilters tests if Get properly gets many objects but with filter values being strings.
func TestGet_WithStringFilters(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should be ignored by Get later on
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Insert data that should be selected by filters
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Get the data from the database
	testStructs, _, err := testCRUD.Get(context.Background(), func() interface{} {
		return &test.TestStruct{}
	}, GetOptions{
		Order:  []string{"Age", "asc", "Price", "asc"},
		Limit:  10,
		Offset: 20,
		Filters: &sqlfilters.Filters{
			"Price": {
				Op:  sqlfilters.OpEqual,
				Val: "444",
			},
			"PrimaryEmail": {
				Op:  sqlfilters.OpEqual,
				Val: "primary@example.com",
			},
			sqlfilters.Raw: {
				Op: sqlfilters.OpAND,
				Val: []interface{}{
					".Price > ? AND .Price NOT IN (?)",
					-200,
					[]int{9999, 9998, 9997},
				},
			},
		},
		ConvertFiltersFromString: true,
	})
	if err != nil {
		t.Fatalf("Get failed to return list of objects when string filters are used: %s", err.(*CRUDError).Op)
	}
	if len(testStructs) != 10 {
		t.Fatalf("Get failed to return list of objects when string filters are used, want %v, got %v", 10, len(testStructs))
	}
	if testStructs[2].(*test.TestStruct).Age != 53 {
		t.Fatalf("Get failed to return correct list of objects when string filters are used, want %v, got %v", 53, testStructs[2].(*test.TestStruct).Age)
	}
}

// TestGetWithoutFilters tests if Get properly gets many objects from the database, without any filters
func TestGet_WithoutFilters(t *testing.T) {
	recreateTestStructTable()

	// Insert data to the database
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Get the data
	testStructs, _, err := testCRUD.Get(context.Background(), func() interface{} {
		return &test.TestStruct{}
	}, GetOptions{
		Order:  []string{"Age", "asc", "Price", "asc"},
		Limit:  13,
		Offset: 14,
	})
	if err != nil {
		t.Fatalf("Get failed to return list of objects: %s", err.(*CRUDError).Op)
	}
	if len(testStructs) != 13 {
		t.Fatalf("Get failed to return list of objects, want %v, got %v", 10, len(testStructs))
	}
	if testStructs[2].(*test.TestStruct).Age != 47 {
		t.Fatalf("Get failed to return correct list of objects, want %v, got %v", 47, testStructs[2].(*test.TestStruct).Age)
	}
}

// TestGetWithRowObjTransformFunc tests if Get can properly return a list of custom elements (eg. string)
// where each object (row from the database) is transform with a specific function
func TestGet_WithRowObjTransformFunc(t *testing.T) {
	recreateTestStructTable()

	// Insert data to the database
	for i := 1; i < 3; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		ts.FirstName = fmt.Sprintf("%s %d", ts.FirstName, i)
		_ = testCRUD.Save(context.Background(), ts, SaveOptions{})
	}

	// Get the data
	testCustomList, _, err := testCRUD.Get(context.Background(), func() interface{} {
		return &test.TestStruct{}
	}, GetOptions{
		Order: []string{"Age", "asc"},
		RowObjTransformFunc: func(obj interface{}) interface{} {
			out := "<tr>"

			v := reflect.ValueOf(obj)
			elem := v.Elem()
			i := reflect.Indirect(v)
			s := i.Type()
			for j := 0; j < s.NumField(); j++ {
				field := s.Field(j)
				fieldType := field.Type.Kind()

				// print only few fields
				if field.Name != "ID" && field.Name != "PrimaryEmail" {
					continue
				}

				out += "<td>"
				if fieldType == reflect.String {
					out += html.EscapeString(elem.Field(j).String())
				}
				if fieldType == reflect.Bool {
					out += fmt.Sprintf("%v", elem.Field(j).Bool())
				}
				if fieldType == reflect.Int || fieldType == reflect.Int64 {
					out += fmt.Sprintf("%d", elem.Field(j).Int())
				}
				if fieldType == reflect.Uint || fieldType == reflect.Uint64 {
					out += fmt.Sprintf("%d", elem.Field(j).Uint())
				}
				out += "</td>"
			}

			out += "</tr>"

			return out
		},
	})
	if err != nil {
		t.Fatalf("Get failed to return list of objects modified with transform func: %s", err.(*CRUDError).Op)
	}
	if len(testCustomList) != 2 {
		t.Fatalf("Get with transform func returned invalid number of objects, wanted %d got %d", 2, len(testCustomList))
	}

	// One of the columns is a random number so testing just the beginning
	if !strings.HasPrefix(testCustomList[0].(string), "<tr><td>1</td><td>primary@example.com</td>") || !strings.HasPrefix(testCustomList[1].(string), "<tr><td>2</td><td>primary@example.com</td>") {
		t.Fatalf("Get with transform func returned invalid objects")
	}
}
