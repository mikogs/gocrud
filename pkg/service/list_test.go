package service

import (
	"context"
	"fmt"
	"html"
	"reflect"
	"strings"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/test"
)

func TestList_WithFilters(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should be ignored by Get later on
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	// Insert data that should be selected by filters
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	// Get the data from the database
	testStructs, _, err := testService.List(context.Background(), "teststruct", 10, 20, "Age", "asc", map[string]string{
		"Price":        "444",
		"PrimaryEmail": "primary@example.com",
	}, map[string]string{
		"Price":        "eq",
		"PrimaryEmail": "eq",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("List failed to return list of objects: %s", err.Error())
	}
	if len(testStructs) != 10 {
		t.Fatalf("List failed to return list of objects, want %v, got %v", 10, len(testStructs))
	}
	if testStructs[2].(*test.TestStruct).Age != 53 {
		t.Fatalf("List failed to return correct list of objects, want %v, got %v", 53, testStructs[2].(*test.TestStruct).Age)
	}
}

func TestList_WithoutFilters(t *testing.T) {
	recreateTestStructTable()

	// Insert data to the database
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	// Get the data
	testStructs, _, err := testService.List(context.Background(), "teststruct", 13, 14, "Age", "asc", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Get failed to return list of objects: %s", err.Error())
	}
	if len(testStructs) != 13 {
		t.Fatalf("Get failed to return list of objects, want %v, got %v", 13, len(testStructs))
	}
	if testStructs[2].(*test.TestStruct).Age != 47 {
		t.Fatalf("Get failed to return correct list of objects, want %v, got %v", 47, testStructs[2].(*test.TestStruct).Age)
	}
}

func TestList_WithRowObjTransformFunc(t *testing.T) {
	recreateTestStructTable()

	// Insert data to the database
	for i := 1; i < 3; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		ts.FirstName = fmt.Sprintf("%s %d", ts.FirstName, i)
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	// Get the data
	testCustomList, _, err := testService.List(context.Background(), "teststruct", 10, 0, "Age", "asc", nil, nil, func(obj interface{}) interface{} {
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
	}, nil, nil)
	if err != nil {
		t.Fatalf("List failed to return list of objects modified with transform func: %s", err.Error())
	}
	if len(testCustomList) != 2 {
		t.Fatalf("List with transform func returned invalid number of objects, wanted %d got %d", 2, len(testCustomList))
	}
	// One of the columns is a random number so testing just the beginning
	if !strings.HasPrefix(testCustomList[0].(string), "<tr><td>1</td><td>primary@example.com</td>") || !strings.HasPrefix(testCustomList[1].(string), "<tr><td>2</td><td>primary@example.com</td>") {
		t.Fatalf("List with transform func returned invalid objects")
	}
}

func TestList_InvalidFilterOp(t *testing.T) {
	_, _, err := testService.List(context.Background(), "teststruct", 10, 0, "", "",
		map[string]string{"Price": "444"},
		map[string]string{"Price": "bogus"},
		nil, nil, nil)
	fatalIfNotFilterViolations(t, err, map[string]string{"Price": gocrud.ViolationType})
}

func TestList_InvalidFilterValue(t *testing.T) {
	_, _, err := testService.List(context.Background(), "teststruct", 10, 0, "", "",
		map[string]string{"Age": "500"},
		map[string]string{"Age": "eq"},
		nil, nil, nil)
	fatalIfNotFilterViolations(t, err, map[string]string{"Age": "max"})
}
