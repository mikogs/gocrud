package gocrud

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/mikogs/gocrud/pkg/gocrudtest"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

func testDeleteMultiple(t *testing.T, crud *CRUD) {
	recreateTwentyTestStructs(t, crud)
	ctx := context.Background()

	_, err := DeleteMultiple[gocrudtest.TestStruct](ctx, crud, DeleteMultipleOptions{
		Filters: sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 500}},
	})
	fatalIfNotViolations(t, err, map[string]string{"Age": "max"})

	ids, err := DeleteMultiple[gocrudtest.TestStruct](ctx, crud, DeleteMultipleOptions{
		Filters: sqlfilters.Filters{"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "another@example.com"}},
	})
	if err != nil || len(ids) != 10 {
		t.Fatalf("DeleteMultiple returned %v, %v, want 10 IDs", ids, err)
	}

	objs, _ := Get[gocrudtest.TestStruct](ctx, crud, GetOptions{Order: []string{"Age", "asc"}, Limit: 1})
	if cnt, _ := GetCount[gocrudtest.TestStruct](ctx, crud, GetCountOptions{}); cnt != 10 || testStructAges(objs)[0] != 31 {
		t.Fatalf("DeleteMultiple left %d objects, the youngest of %v, want 10 aged 31 and more", cnt, testStructAges(objs))
	}
}

func TestDeleteMultiple(t *testing.T)        { testDeleteMultiple(t, testCRUD) }
func TestSQLite_DeleteMultiple(t *testing.T) { testDeleteMultiple(t, testCRUDSQLite) }

func testDeleteMultipleReturnsIDs(t *testing.T, crud *CRUD) {
	createShipments(t, crud)
	ctx := context.Background()

	ids, err := DeleteMultiple[Shipment](ctx, crud, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpIn, Val: []string{"paid", "delivered"}}},
	})
	sort.Slice(ids, func(i, j int) bool { return ids[i].(uint64) < ids[j].(uint64) })
	if err != nil || !reflect.DeepEqual(ids, []interface{}{uint64(2), uint64(4)}) {
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
	sort.Slice(ids, func(i, j int) bool { return ids[i].(uint64) < ids[j].(uint64) })
	if err != nil || !reflect.DeepEqual(ids, []interface{}{uint64(1), uint64(2)}) {
		t.Fatalf("DeleteMultiple returned %v, %v, want the IDs returned by its own query", ids, err)
	}
}

func TestDeleteMultiple_ReturnsIDsWithoutCascaded(t *testing.T) {
	createForeignKeyTables(t, testCRUD)

	ids, err := DeleteMultiple[FkCustomer](context.Background(), testCRUD, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "Acme"}},
	})
	if err != nil || !reflect.DeepEqual(ids, []interface{}{uint64(1)}) {
		t.Fatalf("DeleteMultiple returned %v, %v, want the customer only, without the orders deleted by the foreign key", ids, err)
	}
	if left := orderIDs(t, testCRUD); !reflect.DeepEqual(left, []uint64{3}) {
		t.Fatalf("orders %v are left, want [3]", left)
	}
}
