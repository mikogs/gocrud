package gocrud

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type AggCustomer struct {
	ID      uint64
	Name    string
	Country string
}

type AggOrder struct {
	ID         uint64
	CustomerID uint64
	Status     string
	Total      int64
	Discount   *int64
	PlacedAt   time.Time
	Customer   *AggCustomer `crud:"rel:CustomerID"`
}

func createAggregateData(t *testing.T, crud *CRUD) {
	t.Helper()
	ctx := context.Background()

	for _, obj := range []interface{}{&AggOrder{}, &AggCustomer{}} {
		_ = crud.DropTable(ctx, obj)
	}
	for _, obj := range []interface{}{&AggCustomer{}, &AggOrder{}} {
		if err := crud.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed: %s", err.Error())
		}
	}

	discount := int64(10)
	warsaw := mustLoadLocation(t, "Europe/Warsaw")
	for _, obj := range []interface{}{
		&AggCustomer{Name: "Acme", Country: "PL"},
		&AggCustomer{Name: "Globex", Country: "DE"},
		&AggOrder{CustomerID: 1, Status: "paid", Total: 100, PlacedAt: time.Date(2024, 5, 3, 10, 0, 0, 0, time.UTC)},
		&AggOrder{CustomerID: 1, Status: "paid", Total: 300, Discount: &discount, PlacedAt: time.Date(2024, 5, 20, 10, 0, 0, 0, time.UTC)},
		&AggOrder{CustomerID: 2, Status: "paid", Total: 50, PlacedAt: time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)},
		&AggOrder{CustomerID: 2, Status: "new", Total: 70, PlacedAt: time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)},
		// 30 June in UTC
		&AggOrder{CustomerID: 0, Status: "paid", Total: 20, PlacedAt: time.Date(2024, 7, 1, 1, 0, 0, 0, warsaw)},
	} {
		if err := crud.Save(ctx, obj, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}
}

type salesByStatus struct {
	Status  string  `crud:"group:Status"`
	Orders  int64   `crud:"agg:count"`
	Revenue int64   `crud:"agg:sum:Total"`
	Average float64 `crud:"agg:avg:Total"`
	Largest int64   `crud:"agg:max:Total"`
}

type salesByCountry struct {
	Country string    `crud:"group:Customer.Country"`
	Month   time.Time `crud:"group:PlacedAt:month"`
	Orders  int64     `crud:"agg:count"`
	Revenue int64     `crud:"agg:sum:Total"`
}

type salesTotals struct {
	Orders       int64      `crud:"agg:count"`
	Discounted   int64      `crud:"agg:count:Discount"`
	Discounts    int64      `crud:"agg:sum:Discount"`
	First        time.Time  `crud:"agg:min:PlacedAt"`
	FirstPointer *time.Time `crud:"agg:min:PlacedAt"`
}

type salesByCustomer struct {
	Customer *string `crud:"group:Customer.Name"`
	Discount *int64  `crud:"agg:sum:Discount"`
}

func testAggregate(t *testing.T, crud *CRUD) {
	createAggregateData(t, crud)
	ctx := context.Background()

	byStatus, err := Aggregate[AggOrder, salesByStatus](ctx, crud, AggregateOptions{Order: []string{"Status", "asc"}})
	want := []*salesByStatus{{"new", 1, 70, 70, 70}, {"paid", 4, 470, 117.5, 300}}
	if err != nil || !reflect.DeepEqual(byStatus, want) {
		t.Fatalf("Aggregate by status returned %v, %v, want %v", deref(byStatus), err, deref(want))
	}

	byCountry, err := Aggregate[AggOrder, salesByCountry](ctx, crud, AggregateOptions{
		Filters: sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
		Order:   []string{"Country", "asc", "Month", "asc"},
	})
	may, june := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	wantCountry := []*salesByCountry{{"", june, 1, 20}, {"DE", june, 1, 50}, {"PL", may, 2, 400}}
	if err != nil || !reflect.DeepEqual(byCountry, wantCountry) {
		t.Fatalf("Aggregate by country and month returned %v, %v, want %v", deref(byCountry), err, deref(wantCountry))
	}

	totals, err := Aggregate[AggOrder, salesTotals](ctx, crud, AggregateOptions{})
	first := time.Date(2024, 5, 3, 10, 0, 0, 0, time.UTC)
	if err != nil || len(totals) != 1 || totals[0].Orders != 5 || totals[0].Discounted != 1 || totals[0].Discounts != 10 ||
		!totals[0].First.Equal(first) || totals[0].First.Location() != time.UTC || totals[0].FirstPointer == nil || !totals[0].FirstPointer.Equal(first) {
		t.Fatalf("Aggregate without groups returned %v, %v, want the totals", deref(totals), err)
	}

	byCustomer, err := Aggregate[AggOrder, salesByCustomer](ctx, crud, AggregateOptions{
		Filters: sqlfilters.Filters{"CustomerID": {Op: sqlfilters.OpNotEqual, Val: 1}},
	})
	if err != nil || len(byCustomer) != 2 {
		t.Fatalf("Aggregate by customer returned %v, %v, want 2 groups", deref(byCustomer), err)
	}
	for _, row := range byCustomer {
		if row.Discount != nil || (row.Customer != nil && *row.Customer != "Globex") {
			t.Fatalf("Aggregate by customer returned %+v, want NULL in the pointers without values", *row)
		}
	}
}

func testAggregateHavingAndOrder(t *testing.T, crud *CRUD) {
	createAggregateData(t, crud)
	ctx := context.Background()

	rows, err := Aggregate[AggOrder, salesByStatus](ctx, crud, AggregateOptions{
		Filters: sqlfilters.Filters{"Customer.Country": {Op: sqlfilters.OpIn, Val: []string{"PL", "DE"}}},
		Having:  sqlfilters.Filters{"Revenue": {Op: sqlfilters.OpGreater, Val: 100}, "Orders": {Op: sqlfilters.OpGreaterOrEqual, Val: 2}},
		Order:   []string{"Revenue", "desc"},
	})
	if err != nil || len(rows) != 1 || rows[0].Status != "paid" || rows[0].Revenue != 450 {
		t.Fatalf("Aggregate with Having returned %v, %v, want the paid orders of customers from PL and DE", deref(rows), err)
	}

	rows, err = Aggregate[AggOrder, salesByStatus](ctx, crud, AggregateOptions{Order: []string{"Revenue", "asc"}, Limit: 1, Offset: 1})
	if err != nil || len(rows) != 1 || rows[0].Status != "paid" {
		t.Fatalf("Aggregate with Limit and Offset returned %v, %v, want the second group by revenue", deref(rows), err)
	}
}

func testAggregateInvalid(t *testing.T, crud *CRUD) {
	createAggregateData(t, crud)
	ctx := context.Background()

	check := func(name string, err error, want string) {
		t.Helper()
		if !errors.Is(err, ErrAggregate) || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: Aggregate returned %v, want %v with %q", name, err, ErrAggregate, want)
		}
	}

	_, err := Aggregate[AggOrder, struct {
		Status string
	}](ctx, crud, AggregateOptions{})
	check("a field without a tag", err, "must be group:<field> or agg:<function>")

	_, err = Aggregate[AggOrder, struct {
		Total int64 `crud:"agg:median:Total"`
	}](ctx, crud, AggregateOptions{})
	check("an unknown function", err, `unknown aggregate function "median"`)

	_, err = Aggregate[AggOrder, struct {
		Total int64 `crud:"agg:sum"`
	}](ctx, crud, AggregateOptions{})
	check("sum without a field", err, "agg:sum needs a field")

	_, err = Aggregate[AggOrder, struct {
		Unknown string `crud:"group:Unknown"`
	}](ctx, crud, AggregateOptions{})
	check("an unknown field", err, "Unknown is not a stored field")

	_, err = Aggregate[AggCustomer, struct {
		Name string `crud:"group:Name:month"`
	}](ctx, crud, AggregateOptions{})
	check("a truncated field that is not a time", err, "Name is not a time")

	_, err = Aggregate[AggOrder, struct {
		Month time.Time `crud:"group:PlacedAt:week"`
	}](ctx, crud, AggregateOptions{})
	check("an unknown unit", err, `unknown unit "week"`)

	_, err = Aggregate[AggOrder, struct {
		Name string `crud:"group:Status.Name"`
	}](ctx, crud, AggregateOptions{})
	check("a path through a field that is not a relation", err, "Status is not a belongs-to relation field")

	_, err = Aggregate[AggOrder, salesByStatus](ctx, crud, AggregateOptions{Having: sqlfilters.Filters{"Total": {Op: sqlfilters.OpGreater, Val: 1}}})
	check("Having on a field of the struct", err, "Total is not a field of the result struct")

	_, err = Aggregate[AggOrder, salesByStatus](ctx, crud, AggregateOptions{Order: []string{"Total", "asc"}})
	check("an order on a field of the struct", err, "Total is not a field of the result struct")
}

func deref[R any](rows []*R) []R {
	values := make([]R, len(rows))
	for i, row := range rows {
		values[i] = *row
	}
	return values
}

func TestAggregate(t *testing.T)        { testAggregate(t, testCRUD) }
func TestSQLite_Aggregate(t *testing.T) { testAggregate(t, testCRUDSQLite) }

func TestAggregate_HavingAndOrder(t *testing.T) { testAggregateHavingAndOrder(t, testCRUD) }
func TestSQLite_Aggregate_HavingAndOrder(t *testing.T) {
	testAggregateHavingAndOrder(t, testCRUDSQLite)
}

func TestAggregate_Invalid(t *testing.T)        { testAggregateInvalid(t, testCRUD) }
func TestSQLite_Aggregate_Invalid(t *testing.T) { testAggregateInvalid(t, testCRUDSQLite) }

func TestAggregate_MonthsInUTC(t *testing.T) {
	createAggregateData(t, testCRUD)
	ctx := context.Background()

	err := testCRUD.RunInTx(ctx, func(tx *CRUD) error {
		// in Tokyo, the order of 30 June at 23:00 UTC is placed on 1 July
		if _, err := tx.conn.ExecContext(ctx, "SET LOCAL TIME ZONE 'Asia/Tokyo'"); err != nil {
			return err
		}

		rows, err := Aggregate[AggOrder, salesByCountry](ctx, tx, AggregateOptions{
			Filters: sqlfilters.Filters{"CustomerID": {Op: sqlfilters.OpEqual, Val: 0}},
		})
		if err != nil {
			return err
		}
		if want := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC); len(rows) != 1 || !rows[0].Month.Equal(want) {
			t.Fatalf("Aggregate in another time zone returned %v, want the month %v in UTC", deref(rows), want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunInTx failed: %s", err.Error())
	}
}
