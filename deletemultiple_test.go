package gocrud

import (
	"context"
	"reflect"
	"sort"
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
	_, err := testCRUD.DeleteMultiple(context.Background(), &test.TestStruct{}, DeleteMultipleOptions{
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
	_, err := testCRUD.DeleteMultiple(context.Background(), &test.TestStruct{}, DeleteMultipleOptions{
		Filters: sqlfilters.Or(sqlfilters.Filters{
			"Price": {
				Op:  sqlfilters.OpEqual,
				Val: 444,
			},
			"PrimaryEmail": {
				Op:  sqlfilters.OpEqual,
				Val: "primary@example.com",
			},
		}, sqlfilters.Raw(".Age = ? OR .Age IN (?) OR (.Age = ? AND .PrimaryEmail = ?)", 31, []int{32, 33, 34}, 35, "miko@example.com")),
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
	_, err := testCRUD.DeleteMultiple(context.Background(), &test.TestStruct{}, DeleteMultipleOptions{
		Filters: sqlfilters.Raw("(.Price = ? AND .PrimaryEmail = ?) OR (.Age = ? OR .Age IN (?) OR (.Age = ? AND .PrimaryEmail = ?))", 444, "primary@example.com", 31, []int{32, 33, 34}, 35, "miko@example.com"),
	})
	if err != nil {
		t.Fatalf("DeleteMultiple failed to delete objects: %s", err.(*CRUDError).Op)
	}

	cnt, _ := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{})
	if cnt != 46 {
		t.Fatalf("DeleteMultiple removed invalid number of rows, there are %d rows left, instead of %d", cnt, 46)
	}
}

func TestDeleteMultiple_InvalidFilterValue(t *testing.T) {
	recreateTestStructTable()

	for i := 1; i < 11; i++ {
		_ = testCRUD.Save(context.Background(), test.TestStructWithData(), SaveOptions{})
	}

	_, err := testCRUD.DeleteMultiple(context.Background(), &test.TestStruct{}, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{
			"Age": {Op: sqlfilters.OpEqual, Val: 500},
		},
	})
	fatalIfNotViolations(t, err, map[string]string{"Age": "max"})

	cnt, _ := testCRUD.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{})
	if cnt != 10 {
		t.Fatalf("DeleteMultiple deleted rows using an invalid filter, got %d rows, want 10", cnt)
	}
}

func testDeleteMultipleReturnsIDs(t *testing.T, crud *CRUD) {
	createShipments(t, crud)
	ctx := context.Background()

	ids, err := DeleteMultiple[Shipment](ctx, crud, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpIn, Val: []string{"paid", "delivered"}}},
	})
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if err != nil || !reflect.DeepEqual(ids, []uint64{2, 4}) {
		t.Fatalf("DeleteMultiple returned %v, %v, want [2 4]", ids, err)
	}

	ids, err = DeleteMultiple[Shipment](ctx, crud, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
	})
	if err != nil || ids == nil || len(ids) != 0 {
		t.Fatalf("DeleteMultiple returned %v, %v, want an empty slice when nothing is deleted", ids, err)
	}

	ids, err = DeleteMultiple[Shipment](ctx, crud, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "lost"}},
	})
	fatalIfNotViolations(t, err, map[string]string{"Status": "oneof"})
	if ids != nil {
		t.Fatalf("DeleteMultiple returned %v with an err, want nil", ids)
	}
}

func TestDeleteMultiple_ReturnsIDs(t *testing.T) { testDeleteMultipleReturnsIDs(t, testCRUD) }
func TestSQLite_DeleteMultiple_ReturnsIDs(t *testing.T) {
	testDeleteMultipleReturnsIDs(t, testCRUDSQLite)
}

func TestSQLite_DeleteMultiple_ReturnsIDsOfOwnQuery(t *testing.T) {
	ctx := context.Background()
	_ = DropTable[HookNote](ctx, testCRUDSQLite)
	_ = CreateTable[HookNote](ctx, testCRUDSQLite)
	for _, text := range []string{"a", "b"} {
		_ = testCRUDSQLite.Save(ctx, &HookNote{Text: text}, SaveOptions{})
	}

	ids, err := DeleteMultiple[HookNote](ctx, testCRUDSQLite, DeleteMultipleOptions{})
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if err != nil || !reflect.DeepEqual(ids, []uint64{1, 2}) {
		t.Fatalf("DeleteMultiple returned %v, %v, want the IDs returned by its own query", ids, err)
	}
}

func TestDeleteMultiple_ReturnsIDsWithoutCascaded(t *testing.T) {
	createForeignKeyTables(t, testCRUD)

	ids, err := DeleteMultiple[FkCustomer](context.Background(), testCRUD, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "Acme"}},
	})
	if err != nil || !reflect.DeepEqual(ids, []uint64{1}) {
		t.Fatalf("DeleteMultiple returned %v, %v, want the customer only, without the orders deleted by the foreign key", ids, err)
	}
	if left := orderIDs(t, testCRUD); !reflect.DeepEqual(left, []uint64{3}) {
		t.Fatalf("orders %v are left, want [3]", left)
	}
}
