package gocrud

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

func customerIDs(t *testing.T, crud *CRUD, filter sqlfilters.Filter) []uint64 {
	t.Helper()

	customers, err := Get[AggCustomer](context.Background(), crud, GetOptions{Filters: filter, Order: []string{"ID", "asc"}})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	ids := []uint64{}
	for _, customer := range customers {
		ids = append(ids, customer.ID)
	}
	return ids
}

func testSubqueries(t *testing.T, crud *CRUD) {
	createAggregateData(t, crud)
	ctx := context.Background()

	order := &AggOrder{Status: "unchanged"}
	ids := customerIDs(t, crud, sqlfilters.Filters{
		"Country": {Op: sqlfilters.OpIn, Val: []string{"PL", "DE"}},
		"ID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(order, "CustomerID", sqlfilters.Filters{
			"Status": {Op: sqlfilters.OpEqual, Val: "paid"},
			"Total":  {Op: sqlfilters.OpGreaterOrEqual, Val: 200},
		})},
		"Name": {Op: sqlfilters.OpNotEqual, Val: "Initech"},
	})
	if !reflect.DeepEqual(ids, []uint64{1}) || order.Status != "unchanged" {
		t.Fatalf("Get of the customers with large paid orders returned %v, and changed the object to %+v, want [1]", ids, *order)
	}

	ids = customerIDs(t, crud, sqlfilters.Or(
		sqlfilters.Filters{"ID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&AggOrder{}, "CustomerID", sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "new"}})}},
		sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "Acme"}},
	))
	if !reflect.DeepEqual(ids, []uint64{1, 2}) {
		t.Fatalf("Get with a subquery in OR returned %v, want [1 2]", ids)
	}

	ids = customerIDs(t, crud, sqlfilters.Filters{"ID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&AggOrder{}, "CustomerID", sqlfilters.Filters{
		"PlacedAt": {Op: sqlfilters.OpGreaterOrEqual, Val: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
	})}})
	if !reflect.DeepEqual(ids, []uint64{2}) {
		t.Fatalf("Get with a time in a subquery returned %v, want [2]", ids)
	}

	customers, err := Get[AggCustomer](ctx, crud, GetOptions{ConvertFiltersFromString: true, Filters: sqlfilters.Filters{
		"Country": {Op: sqlfilters.OpEqual, Val: "DE"},
		"ID":      {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&AggOrder{}, "CustomerID", sqlfilters.Filters{"Total": {Op: sqlfilters.OpGreater, Val: 60}})},
	}})
	if err != nil || len(customers) != 1 || customers[0].ID != 2 {
		t.Fatalf("Get with values from strings and a subquery returned %d customers, %v, want [2]", len(customers), err)
	}

	sameTotal := sqlfilters.Filters{"Total": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&AggOrder{}, "Total", sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "new"}})}}
	if count, err := GetCount[AggOrder](ctx, crud, GetCountOptions{Filters: sameTotal}); err != nil || count != 1 {
		t.Fatalf("GetCount with a subquery of the same struct returned %d, %v, want 1", count, err)
	}

	fromPL := sqlfilters.Filters{"CustomerID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&AggCustomer{}, "ID", sqlfilters.Filters{"Country": {Op: sqlfilters.OpEqual, Val: "PL"}})}}
	sales, err := Aggregate[AggOrder, salesByStatus](ctx, crud, AggregateOptions{Filters: fromPL})
	if err != nil || len(sales) != 1 || sales[0].Status != "paid" || sales[0].Revenue != 400 {
		t.Fatalf("Aggregate with a subquery returned %v, %v, want the paid orders of the customer from PL", deref(sales), err)
	}

	if err := UpdateMultiple[AggOrder](ctx, crud, map[string]interface{}{"Status": "archived"}, UpdateMultipleOptions{Filters: fromPL}); err != nil {
		t.Fatalf("UpdateMultiple with a subquery failed: %s", err.Error())
	}
	archived := sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "archived"}}
	if count, err := GetCount[AggOrder](ctx, crud, GetCountOptions{Filters: archived}); err != nil || count != 2 {
		t.Fatalf("GetCount of the updated orders returned %d, %v, want 2", count, err)
	}

	deleted, err := DeleteMultiple[AggOrder](ctx, crud, DeleteMultipleOptions{Filters: fromPL})
	if err != nil || len(deleted) != 2 {
		t.Fatalf("DeleteMultiple with a subquery returned %v, %v, want 2 orders", deleted, err)
	}
}

func testSubqueriesInvalid(t *testing.T, crud *CRUD) {
	createAggregateData(t, crud)
	ctx := context.Background()

	tests := map[string]struct {
		val  sqlfilters.OpVal
		want string
	}{
		"not OpIn":     {sqlfilters.OpVal{Op: sqlfilters.OpEqual, Val: sqlfilters.Subquery(&AggOrder{}, "CustomerID", nil)}, "can only be the value of OpIn"},
		"not a struct": {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: sqlfilters.Subquery("orders", "CustomerID", nil)}, "string is not a struct"},
		"not stored":   {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&AggOrder{}, "Customer", nil)}, "Customer is not a stored field of AggOrder"},
		"of another type": {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&AggOrder{}, "Status", nil)},
			"AggOrder.Status (string) cannot be compared with ID (uint64)"},
		"with an invalid path": {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&AggOrder{}, "CustomerID", sqlfilters.Filters{
			"Status.Name": {Op: sqlfilters.OpEqual, Val: "x"},
		})}, "Status is not a relation field of AggOrder"},
	}
	for name, test := range tests {
		_, err := Get[AggCustomer](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"ID": test.val}})
		if !errors.Is(err, ErrSubquery) || !strings.Contains(err.Error(), "ID: ") || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("Get with a subquery that is %s returned %v, want %v with %q", name, err, ErrSubquery, test.want)
		}
	}

	invalidValue := sqlfilters.Filters{"ID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&AggOrder{}, "CustomerID", sqlfilters.Filters{
		"Total": {Op: sqlfilters.OpEqual, Val: nil},
	})}}
	_, err := Get[AggCustomer](ctx, crud, GetOptions{Filters: invalidValue})
	if !errors.Is(err, ErrSubquery) {
		t.Fatalf("Get with an invalid value in a subquery returned %v, want %v", err, ErrSubquery)
	}
	fatalIfNotViolations(t, err, map[string]string{"Total": ViolationType})

	if err := crud.ValidateFilters(&AggCustomer{}, invalidValue); !errors.Is(err, ErrSubquery) {
		t.Fatalf("ValidateFilters with an invalid value in a subquery returned %v, want %v", err, ErrSubquery)
	}

	_, err = Aggregate[AggOrder, salesByStatus](ctx, crud, AggregateOptions{Having: sqlfilters.Filters{
		"Orders": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&AggOrder{}, "Total", nil)},
	}})
	if !strings.Contains(err.Error(), "subqueries cannot be used here") {
		t.Fatalf("Aggregate with a subquery in Having returned %v, want an error", err)
	}
}

func testSubqueriesUUID(t *testing.T, crud *CRUD) {
	data := createUUIDRelationTables(t, crud)
	ctx := context.Background()

	authors, err := Get[UAuthor](ctx, crud, GetOptions{Filters: sqlfilters.Filters{
		"ID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&UPost{}, "AuthorID", sqlfilters.Filters{
			"ID": {Op: sqlfilters.OpEqual, Val: strings.ToUpper(data.posts[2].ID)},
		})},
	}})
	if err != nil || len(authors) != 1 || authors[0].ID != data.bob.ID {
		t.Fatalf("Get by the uppercase UUID in a subquery returned %d authors, %v, want Bob", len(authors), err)
	}

	posts, err := Get[UPost](ctx, crud, GetOptions{Filters: sqlfilters.Filters{
		"AuthorID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&UAuthor{}, "ID", sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "Alice"}})},
	}})
	if err != nil || len(posts) != 2 {
		t.Fatalf("Get of the posts with a subquery of UUIDs returned %d posts, %v, want 2", len(posts), err)
	}
}

func TestSubqueries(t *testing.T)        { testSubqueries(t, testCRUD) }
func TestSQLite_Subqueries(t *testing.T) { testSubqueries(t, testCRUDSQLite) }

func TestSubqueries_Invalid(t *testing.T)        { testSubqueriesInvalid(t, testCRUD) }
func TestSQLite_Subqueries_Invalid(t *testing.T) { testSubqueriesInvalid(t, testCRUDSQLite) }

func TestSubqueries_UUID(t *testing.T)        { testSubqueriesUUID(t, testCRUD) }
func TestSQLite_Subqueries_UUID(t *testing.T) { testSubqueriesUUID(t, newSQLiteForeignKeysCRUD(t)) }
