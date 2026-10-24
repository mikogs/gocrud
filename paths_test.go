package gocrud

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

func preOrderIDs(t *testing.T, crud *CRUD, filter sqlfilters.Filter, convert bool) []uint64 {
	t.Helper()

	orders, err := Get[PreOrder](context.Background(), crud, GetOptions{Order: []string{"ID", "asc"}, Filters: filter, ConvertFiltersFromString: convert})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}

	ids := []uint64{}
	for _, order := range orders {
		ids = append(ids, order.ID)
	}
	return ids
}

func preCustomerIDs(t *testing.T, crud *CRUD, filter sqlfilters.Filter) []uint64 {
	t.Helper()

	customers, err := Get[PreCustomer](context.Background(), crud, GetOptions{Order: []string{"ID", "asc"}, Filters: filter})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}

	ids := []uint64{}
	for _, customer := range customers {
		ids = append(ids, customer.ID)
	}
	return ids
}

func testFilterPaths(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)

	orders := map[string]struct {
		filter sqlfilters.Filter
		want   []uint64
	}{
		"belongs to":                     {sqlfilters.Filters{"Customer.Name": {Op: sqlfilters.OpEqual, Val: "Acme"}}, []uint64{1, 2}},
		"belongs to that does not exist": {sqlfilters.Filters{"Shipper.Name": {Op: sqlfilters.OpLike, Val: "%"}}, []uint64{1}},
		"with fields around the path": {sqlfilters.Filters{
			"CustomerID":    {Op: sqlfilters.OpNotEqual, Val: 0},
			"Customer.Name": {Op: sqlfilters.OpEqual, Val: "Acme"},
			"Status":        {Op: sqlfilters.OpEqual, Val: "paid"},
		}, []uint64{2}},
		"in or": {sqlfilters.Or(
			sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "draft"}},
			sqlfilters.Filters{"Customer.Name": {Op: sqlfilters.OpEqual, Val: "Globex"}},
		), []uint64{3, 4}},
		"has many": {sqlfilters.Filters{"Items.Product": {Op: sqlfilters.OpIn, Val: []string{"Cable", "Mouse"}}}, []uint64{1, 2}},
	}
	for name, test := range orders {
		t.Run("orders "+name, func(t *testing.T) {
			if got := preOrderIDs(t, crud, test.filter, false); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Get returned orders %v, want %v", got, test.want)
			}
		})
	}

	customers := map[string]struct {
		filter sqlfilters.Filter
		want   []uint64
	}{
		"has many": {sqlfilters.Filters{"Orders.Status": {Op: sqlfilters.OpEqual, Val: "placed"}}, []uint64{1, 2}},
		"one related row": {sqlfilters.Filters{
			"Orders.Status":       {Op: sqlfilters.OpEqual, Val: "paid"},
			"Orders.Shipper.Name": {Op: sqlfilters.OpEqual, Val: "DHL"},
		}, []uint64{}},
		"any related rows": {sqlfilters.And(
			sqlfilters.Filters{"Orders.Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
			sqlfilters.Filters{"Orders.Shipper.Name": {Op: sqlfilters.OpEqual, Val: "DHL"}},
		), []uint64{1}},
		"nested":               {sqlfilters.Filters{"Orders.Items.Product": {Op: sqlfilters.OpEqual, Val: "Mouse"}}, []uint64{1}},
		"without related rows": {sqlfilters.Filters{"Orders.Status": {Op: sqlfilters.OpEqual, Val: "cancelled"}}, []uint64{}},
	}
	for name, test := range customers {
		t.Run("customers "+name, func(t *testing.T) {
			if got := preCustomerIDs(t, crud, test.filter); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Get returned customers %v, want %v", got, test.want)
			}
		})
	}

	cnt, err := GetCount[PreCustomer](context.Background(), crud, GetCountOptions{
		Filters: sqlfilters.Filters{"Orders.Status": {Op: sqlfilters.OpEqual, Val: "placed"}},
	})
	if err != nil || cnt != 2 {
		t.Fatalf("GetCount returned %d, %v, want 2", cnt, err)
	}
}

func testFilterPathsFromStrings(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)

	filter := sqlfilters.Filters{
		"Customer.ID":   {Op: sqlfilters.OpIn, Val: "1,2"},
		"Items.Product": {Op: sqlfilters.OpEqual, Val: "Keyboard"},
	}
	if got := preOrderIDs(t, crud, filter, true); !reflect.DeepEqual(got, []uint64{1}) {
		t.Fatalf("Get with string values returned orders %v, want [1]", got)
	}

	_, err := Get[PreOrder](context.Background(), crud, GetOptions{
		Filters:                  sqlfilters.Filters{"Customer.ID": {Op: sqlfilters.OpEqual, Val: "first"}},
		ConvertFiltersFromString: true,
	})
	fatalIfNotViolations(t, err, map[string]string{"Customer.ID": ViolationType})
}

func testFilterPathsUpdateAndDelete(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)
	ctx := context.Background()

	err := UpdateMultiple[PreOrder](ctx, crud, map[string]interface{}{"Status": "priority"}, UpdateMultipleOptions{
		Filters: sqlfilters.Filters{"Customer.Name": {Op: sqlfilters.OpEqual, Val: "Globex"}},
	})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	if got := preOrderIDs(t, crud, sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "priority"}}, false); !reflect.DeepEqual(got, []uint64{3}) {
		t.Fatalf("UpdateMultiple updated orders %v, want [3]", got)
	}

	deleted, err := DeleteMultiple[PreOrder](ctx, crud, DeleteMultipleOptions{
		Filters: sqlfilters.Filters{"Items.Product": {Op: sqlfilters.OpIn, Val: []string{"Mouse", "Cable"}}},
	})
	sort.Slice(deleted, func(i, j int) bool { return deleted[i].(uint64) < deleted[j].(uint64) })
	if err != nil || !reflect.DeepEqual(deleted, []interface{}{uint64(1), uint64(2)}) {
		t.Fatalf("DeleteMultiple returned %v, %v, want [1 2]", deleted, err)
	}
}

type PathTeam struct {
	ID   uint64
	Name string `validate:"max=5"`
}

type PathMember struct {
	ID     uint64
	TeamID uint64
	Team   *PathTeam `crud:"rel:TeamID"`
}

func testFilterPathsValidation(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	for _, obj := range []interface{}{&PathMember{}, &PathTeam{}} {
		_ = crud.DropTable(ctx, obj)
		if err := crud.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed: %s", err.Error())
		}
	}

	_, err := Get[PathMember](ctx, crud, GetOptions{Filters: sqlfilters.Or(
		sqlfilters.Filters{"Team.Name": {Op: sqlfilters.OpEqual, Val: "much too long"}},
		sqlfilters.Filters{"TeamID": {Op: sqlfilters.OpEqual, Val: nil}},
	)})
	fatalIfNotViolations(t, err, map[string]string{"Team.Name": "max", "TeamID": ViolationType})

	for name, path := range map[string]string{
		"a field that is not a relation": "TeamID.Name",
		"a nested field that is not one": "Team.Name.Length",
	} {
		_, err := Get[PathMember](ctx, crud, GetOptions{Filters: sqlfilters.Filters{path: {Op: sqlfilters.OpEqual, Val: "x"}}})
		if !errors.Is(err, ErrFilterPath) {
			t.Fatalf("Get with %s returned %v, want %v", name, err, ErrFilterPath)
		}
	}

	if err := crud.ValidateFilters(&PathMember{}, sqlfilters.Filters{"Team.Name": {Op: sqlfilters.OpEqual, Val: "ok"}}); err != nil {
		t.Fatalf("ValidateFilters of a valid path returned %v", err)
	}
}

func testFilterPathsUUID(t *testing.T, crud *CRUD) {
	data := createUUIDRelationTables(t, crud)
	ctx := context.Background()

	posts, err := Get[UPost](ctx, crud, GetOptions{
		Filters: sqlfilters.Filters{"Author.ID": {Op: sqlfilters.OpEqual, Val: strings.ToUpper(data.alice.ID)}},
	})
	if err != nil || len(posts) != 2 {
		t.Fatalf("Get by the uppercase UUID of the author returned %d posts, %v, want 2", len(posts), err)
	}

	_, err = Get[UPost](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"Author.ID": {Op: sqlfilters.OpEqual, Val: "42"}}})
	fatalIfNotViolations(t, err, map[string]string{"Author.ID": ViolationType})
}

func testOrderByPaths(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)
	ctx := context.Background()

	tests := map[string]struct {
		options GetOptions
		want    []uint64
	}{
		"ascending":                        {GetOptions{Order: []string{"Customer.Name", "asc", "ID", "asc"}}, []uint64{1, 2, 3, 4}},
		"descending, without one last":     {GetOptions{Order: []string{"Customer.Name", "desc", "ID", "asc"}}, []uint64{3, 1, 2, 4}},
		"related objects that are missing": {GetOptions{Order: []string{"Shipper.Name", "desc", "ID", "asc"}}, []uint64{1, 2, 3, 4}},
		"with filters and pagination": {GetOptions{
			Order:   []string{"Customer.Name", "desc", "ID", "desc"},
			Filters: sqlfilters.Filters{"Items.Product": {Op: sqlfilters.OpLike, Val: "%"}},
			Limit:   1,
			Offset:  1,
		}, []uint64{1}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			orders, err := Get[PreOrder](ctx, crud, test.options)
			if err != nil {
				t.Fatalf("Get failed: %s", err.Error())
			}
			ids := []uint64{}
			for _, order := range orders {
				ids = append(ids, order.ID)
			}
			if !reflect.DeepEqual(ids, test.want) {
				t.Fatalf("Get returned orders %v, want %v", ids, test.want)
			}
		})
	}

	_, err := Get[PreCustomer](ctx, crud, GetOptions{Order: []string{"Orders.Status", "asc"}})
	if !errors.Is(err, ErrInvalidOrder) || !strings.Contains(err.Error(), "Orders is a has-many relation") {
		t.Fatalf("Get ordered by a has-many relation returned %v, want %v", err, ErrInvalidOrder)
	}
}

func TestOrderByPaths(t *testing.T)        { testOrderByPaths(t, testCRUD) }
func TestSQLite_OrderByPaths(t *testing.T) { testOrderByPaths(t, testCRUDSQLite) }

func TestFilterPaths(t *testing.T)        { testFilterPaths(t, testCRUD) }
func TestSQLite_FilterPaths(t *testing.T) { testFilterPaths(t, testCRUDSQLite) }

func TestFilterPaths_FromStrings(t *testing.T)        { testFilterPathsFromStrings(t, testCRUD) }
func TestSQLite_FilterPaths_FromStrings(t *testing.T) { testFilterPathsFromStrings(t, testCRUDSQLite) }

func TestFilterPaths_UpdateAndDelete(t *testing.T) { testFilterPathsUpdateAndDelete(t, testCRUD) }
func TestSQLite_FilterPaths_UpdateAndDelete(t *testing.T) {
	testFilterPathsUpdateAndDelete(t, testCRUDSQLite)
}

func TestFilterPaths_Validation(t *testing.T)        { testFilterPathsValidation(t, testCRUD) }
func TestSQLite_FilterPaths_Validation(t *testing.T) { testFilterPathsValidation(t, testCRUDSQLite) }

func TestFilterPaths_UUID(t *testing.T)        { testFilterPathsUUID(t, testCRUD) }
func TestSQLite_FilterPaths_UUID(t *testing.T) { testFilterPathsUUID(t, newSQLiteForeignKeysCRUD(t)) }
