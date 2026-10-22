package gocrud

import (
	"context"
	"reflect"
	"sort"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

func testFilterGroups(t *testing.T, crud *CRUD) {
	createShipments(t, crud)

	tests := map[string]struct {
		filter sqlfilters.Filter
		want   []uint64
	}{
		"or": {sqlfilters.Or(
			sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "new"}},
			sqlfilters.Filters{"Carrier": {Op: sqlfilters.OpEqual, Val: "ups"}},
		), []uint64{1, 3}},
		"range on one field": {sqlfilters.And(
			sqlfilters.Filters{"Weight": {Op: sqlfilters.OpGreaterOrEqual, Val: 2}},
			sqlfilters.Filters{"Weight": {Op: sqlfilters.OpLower, Val: 4}},
		), []uint64{2, 3}},
		"or with null and in": {sqlfilters.Or(
			&sqlfilters.Filters{"Carrier": {Op: sqlfilters.OpEqual, Val: nil}},
			&sqlfilters.Filters{"Status": {Op: sqlfilters.OpIn, Val: []string{"delivered"}}},
		), []uint64{1, 4}},
		"nested": {sqlfilters.And(
			sqlfilters.Filters{"Weight": {Op: sqlfilters.OpGreater, Val: 1}},
			sqlfilters.Or(
				sqlfilters.Filters{"Carrier": {Op: sqlfilters.OpEqual, Val: "dhl"}},
				sqlfilters.And(sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "shipped"}}, sqlfilters.Raw(".Weight < ?", 10)),
			),
		), []uint64{2, 3, 4}},
		"empty or":  {sqlfilters.Or(), []uint64{}},
		"empty and": {sqlfilters.And(), []uint64{1, 2, 3, 4}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fatalIfNotShipments(t, crud, GetOptions{Filters: test.filter}, test.want)
		})
	}
}

func testFilterGroupsFromStrings(t *testing.T, crud *CRUD) {
	createShipments(t, crud)

	fatalIfNotShipments(t, crud, GetOptions{
		Filters: sqlfilters.Or(
			sqlfilters.Filters{"Weight": {Op: sqlfilters.OpGreater, Val: "3"}},
			&sqlfilters.Filters{"Status": {Op: sqlfilters.OpIn, Val: "new,paid"}},
			sqlfilters.And(sqlfilters.Filters{"Carrier": {Op: sqlfilters.OpEqual, Val: ""}}),
		),
		ConvertFiltersFromString: true,
	}, []uint64{1, 2, 4})

	_, err := Get[Shipment](context.Background(), crud, GetOptions{
		Filters: sqlfilters.Or(
			sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "new"}},
			sqlfilters.Filters{"Weight": {Op: sqlfilters.OpEqual, Val: "heavy"}},
		),
		ConvertFiltersFromString: true,
	})
	fatalIfNotViolations(t, err, map[string]string{"Weight": ViolationType})
}

func testFilterGroupsValidation(t *testing.T, crud *CRUD) {
	createShipments(t, crud)

	_, err := Get[Shipment](context.Background(), crud, GetOptions{
		Filters: sqlfilters.Or(
			sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "new"}},
			sqlfilters.And(
				sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "lost"}},
				sqlfilters.Filters{"Weight": {Op: sqlfilters.OpEqual, Val: nil}},
			),
			sqlfilters.Filters{"ID": {Op: sqlfilters.OpIn, Val: []string{"1"}}},
		),
	})
	fatalIfNotViolations(t, err, map[string]string{"Status": "oneof", "Weight": ViolationType, "ID": ViolationType})
}

func testFilterGroupsUpdateAndDelete(t *testing.T, crud *CRUD) {
	createShipments(t, crud)
	ctx := context.Background()

	err := UpdateMultiple[Shipment](ctx, crud, map[string]interface{}{"Status": "paid"}, UpdateMultipleOptions{
		Filters: sqlfilters.Or(
			sqlfilters.Filters{"Weight": {Op: sqlfilters.OpEqual, Val: 1}},
			sqlfilters.Filters{"Carrier": {Op: sqlfilters.OpEqual, Val: "ups"}},
		),
	})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	fatalIfNotShipments(t, crud, GetOptions{
		Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
	}, []uint64{1, 2, 3})

	deletedIDs, err := DeleteMultiple[Shipment](ctx, crud, DeleteMultipleOptions{
		Filters: sqlfilters.Or(
			sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}, "Weight": {Op: sqlfilters.OpLower, Val: 2}},
			sqlfilters.Raw(".Weight > ?", 3),
		),
	})
	sort.Slice(deletedIDs, func(i, j int) bool { return deletedIDs[i] < deletedIDs[j] })
	if err != nil || !reflect.DeepEqual(deletedIDs, []uint64{1, 4}) {
		t.Fatalf("DeleteMultiple returned %v, %v, want [1 4]", deletedIDs, err)
	}
	fatalIfNotShipments(t, crud, GetOptions{}, []uint64{2, 3})
}

func TestFilterGroups(t *testing.T)        { testFilterGroups(t, testCRUD) }
func TestSQLite_FilterGroups(t *testing.T) { testFilterGroups(t, testCRUDSQLite) }

func TestFilterGroups_FromStrings(t *testing.T) { testFilterGroupsFromStrings(t, testCRUD) }
func TestSQLite_FilterGroups_FromStrings(t *testing.T) {
	testFilterGroupsFromStrings(t, testCRUDSQLite)
}

func TestFilterGroups_Validation(t *testing.T)        { testFilterGroupsValidation(t, testCRUD) }
func TestSQLite_FilterGroups_Validation(t *testing.T) { testFilterGroupsValidation(t, testCRUDSQLite) }

func TestFilterGroups_UpdateAndDelete(t *testing.T) { testFilterGroupsUpdateAndDelete(t, testCRUD) }
func TestSQLite_FilterGroups_UpdateAndDelete(t *testing.T) {
	testFilterGroupsUpdateAndDelete(t, testCRUDSQLite)
}
