package gocrud

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud/internal/fixtures"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

func testGet(t *testing.T, crud *CRUD) {
	recreateTwentyTestStructs(t, crud)
	ctx := context.Background()

	tests := map[string]struct {
		options GetOptions
		want    []int
	}{
		"filters, order and pagination": {GetOptions{
			Order:  []string{"Age", "desc"},
			Limit:  3,
			Offset: 2,
			Filters: sqlfilters.And(
				sqlfilters.Filters{"Price": {Op: sqlfilters.OpEqual, Val: 444}, "PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "primary@example.com"}},
				sqlfilters.Raw(".Age NOT IN (?)", []int{38}),
			),
		}, []int{37, 36, 35}},
		"values from strings": {GetOptions{
			Order:                    []string{"Age", "desc"},
			Limit:                    3,
			Filters:                  sqlfilters.Filters{"Price": {Op: sqlfilters.OpEqual, Val: "444"}, "Age": {Op: sqlfilters.OpLower, Val: "30"}},
			ConvertFiltersFromString: true,
		}, []int{29, 28, 27}},
		"without filters": {GetOptions{
			Order:  []string{"Age", "asc"},
			Limit:  4,
			Offset: 3,
		}, []int{24, 25, 26, 27}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			objs, err := Get[fixtures.TestStruct](ctx, crud, tc.options)
			if got := testStructAges(objs); err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Get returned ages %v, %v, want %v", got, err, tc.want)
			}
		})
	}

	cnt, err := GetCount[fixtures.TestStruct](ctx, crud, GetCountOptions{
		Filters: sqlfilters.Filters{"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "primary@example.com"}},
	})
	if err != nil || cnt != 10 {
		t.Fatalf("GetCount returned %d, %v, want 10", cnt, err)
	}
}

func testGetRowObjTransformFunc(t *testing.T, crud *CRUD) {
	recreateTestStructs(t, crud, 2, func(int) *fixtures.TestStruct { return fixtures.TestStructWithData() })

	rows, _, err := crud.Get(context.Background(), func() interface{} { return &fixtures.TestStruct{} }, GetOptions{
		Order: []string{"ID", "asc"},
		RowObjTransformFunc: func(obj interface{}) interface{} {
			return fmt.Sprintf("%d:%s", obj.(*fixtures.TestStruct).ID, obj.(*fixtures.TestStruct).FirstName)
		},
	})
	if err != nil || !reflect.DeepEqual(rows, []interface{}{"1:John", "2:John"}) {
		t.Fatalf("Get returned %v, %v, want the transformed rows", rows, err)
	}
}

func testGetInvalidFilters(t *testing.T, crud *CRUD) {
	recreateTestStructs(t, crud, 0, nil)
	ctx := context.Background()

	tests := map[string]struct {
		filter  sqlfilters.Filter
		convert bool
		want    string
	}{
		"value that breaks a rule":    {sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 500}}, false, "max"},
		"string that is not a number": {sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: "notanint"}}, true, ViolationType},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Get[fixtures.TestStruct](ctx, crud, GetOptions{Filters: tc.filter, ConvertFiltersFromString: tc.convert})
			fatalIfNotViolations(t, err, map[string]string{"Age": tc.want})

			_, err = GetCount[fixtures.TestStruct](ctx, crud, GetCountOptions{Filters: tc.filter, ConvertFiltersFromString: tc.convert})
			fatalIfNotViolations(t, err, map[string]string{"Age": tc.want})
		})
	}
}

func testUpdateMultiple(t *testing.T, crud *CRUD) {
	recreateTwentyTestStructs(t, crud)
	ctx := context.Background()

	err := UpdateMultiple[fixtures.TestStruct](ctx, crud, map[string]interface{}{"PrimaryEmail": "new@example.com", "Age": 98}, UpdateMultipleOptions{
		Filters: sqlfilters.Filters{"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "another@example.com"}},
	})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}

	cnt, _ := GetCount[fixtures.TestStruct](ctx, crud, GetCountOptions{
		Filters: sqlfilters.Filters{"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "new@example.com"}, "Age": {Op: sqlfilters.OpEqual, Val: 98}},
	})
	if cnt != 10 {
		t.Fatalf("UpdateMultiple updated %d objects, want 10", cnt)
	}

	err = UpdateMultiple[fixtures.TestStruct](ctx, crud, map[string]interface{}{"FirstName": "x", "Age": "notanint"}, UpdateMultipleOptions{})
	fatalIfNotViolations(t, err, map[string]string{"FirstName": "min", "Age": ViolationType})

	if cnt, _ := GetCount[fixtures.TestStruct](ctx, crud, GetCountOptions{Filters: sqlfilters.Filters{"FirstName": {Op: sqlfilters.OpEqual, Val: "John"}}}); cnt != 20 {
		t.Fatalf("UpdateMultiple with invalid values changed objects, %d are unchanged, want 20", cnt)
	}
}

func TestGet(t *testing.T)        { testGet(t, testCRUD) }
func TestSQLite_Get(t *testing.T) { testGet(t, testCRUDSQLite) }

func TestGet_RowObjTransformFunc(t *testing.T)        { testGetRowObjTransformFunc(t, testCRUD) }
func TestSQLite_Get_RowObjTransformFunc(t *testing.T) { testGetRowObjTransformFunc(t, testCRUDSQLite) }

func TestGet_InvalidFilters(t *testing.T)        { testGetInvalidFilters(t, testCRUD) }
func TestSQLite_Get_InvalidFilters(t *testing.T) { testGetInvalidFilters(t, testCRUDSQLite) }

func TestUpdateMultiple(t *testing.T)        { testUpdateMultiple(t, testCRUD) }
func TestSQLite_UpdateMultiple(t *testing.T) { testUpdateMultiple(t, testCRUDSQLite) }

func TestGet_InvalidOrder(t *testing.T) {
	createPreloadData(t, testCRUDSQLite)

	tests := map[string][]string{
		"unknown field":            {"Unknown", "asc"},
		"relation field":           {"Customer", "asc"},
		"has many":                 {"Items.Product", "asc"},
		"not a relation":           {"Status.Name", "asc"},
		"unknown field of related": {"Customer.Unknown", "asc"},
		"no direction":             {"ID"},
	}
	for name, order := range tests {
		if _, err := Get[PreOrder](context.Background(), testCRUDSQLite, GetOptions{Order: order}); !errors.Is(err, ErrInvalidOrder) {
			t.Fatalf("Get ordered by %s returned %v, want %v", name, err, ErrInvalidOrder)
		}
	}

	if _, err := Get[PreOrder](context.Background(), testCRUDSQLite, GetOptions{Order: []string{"Customer.Name", "asc", "ID", "desc"}}); err != nil {
		t.Fatalf("Get ordered by valid fields returned %v", err)
	}
}
