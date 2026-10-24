package gocrud

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mikogs/gocrud/internal/dialect"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type PaymentTotals struct {
	Note      sql.NullString `crud:"group:Note"`
	Total     Money          `crud:"agg:sum:Amount"`
	Refunds   Money          `crud:"agg:sum:Refund"`
	MaxRefund *Money         `crud:"agg:max:Refund"`
	Count     sql.NullInt64  `crud:"agg:sum:Count"`
	Last      sql.NullTime   `crud:"agg:max:PaidAt"`
	LastOrNil *sql.NullTime  `crud:"agg:max:PaidAt"`
	Payments  int64          `crud:"agg:count"`
}

type PaymentsByMonth struct {
	Month sql.NullTime `crud:"group:PaidAt:month"`
	Total Money        `crud:"agg:sum:Amount"`
}

type PaymentsByRef struct {
	Ref      uuid.UUID `crud:"group:Ref"`
	Payments int64     `crud:"agg:count"`
}

func testValuerAggregate(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[Payment](ctx, crud)
	if err := CreateTable[Payment](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	january := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	february := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	refunds := []Money{{cents: 500}, {cents: 200}}
	payments := []*Payment{
		{Amount: Money{cents: 1000}, Note: sql.NullString{String: "a", Valid: true}, Count: sql.NullInt64{Int64: 1, Valid: true},
			PaidAt: sql.NullTime{Time: january, Valid: true}},
		{Amount: Money{cents: 2550}, Refund: &refunds[0], PaidAt: sql.NullTime{Time: january.Add(time.Hour), Valid: true}},
		{Amount: Money{cents: 725}, Note: sql.NullString{String: "b", Valid: true}, Count: sql.NullInt64{Int64: 3, Valid: true}},
		{Amount: Money{cents: 100}, Refund: &refunds[1], PaidAt: sql.NullTime{Time: february, Valid: true}},
	}
	for _, payment := range payments {
		payment.Ref = uuid.New()
		if err := crud.Save(ctx, payment, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	totals, err := Aggregate[Payment, PaymentTotals](ctx, crud, AggregateOptions{Order: []string{"Total", "desc"}})
	if err != nil || len(totals) != 3 {
		t.Fatalf("Aggregate returned %d results, %v, want 3", len(totals), err)
	}
	withoutNote, a, b := totals[0], totals[1], totals[2]
	if withoutNote.Note.Valid || withoutNote.Total.cents != 2650 || withoutNote.Refunds.cents != 700 || withoutNote.MaxRefund.cents != 500 ||
		withoutNote.Count.Valid || !withoutNote.Last.Time.Equal(february) || !withoutNote.LastOrNil.Time.Equal(february) || withoutNote.Payments != 2 {
		t.Fatalf("Aggregate returned %+v for the payments without a note", withoutNote)
	}
	if a.Note.String != "a" || a.Total.cents != 1000 || a.Refunds.cents != 0 || a.MaxRefund != nil || a.Count.Int64 != 1 || !a.Last.Time.Equal(january) {
		t.Fatalf("Aggregate returned %+v for the note a", a)
	}
	if b.Note.String != "b" || b.Total.cents != 725 || b.Last.Valid || b.LastOrNil != nil || b.Count.Int64 != 3 {
		t.Fatalf("Aggregate returned %+v for the note b", b)
	}

	// a number, which SQLite compares with the sums as a number, and not the text that a decimal is written as
	expensive, err := Aggregate[Payment, PaymentTotals](ctx, crud, AggregateOptions{
		Having: sqlfilters.Filters{"Total": {Op: sqlfilters.OpGreater, Val: 9.0}},
		Order:  []string{"Total", "desc"},
	})
	if err != nil || len(expensive) != 2 || expensive[0].Total.cents != 2650 || expensive[1].Total.cents != 1000 {
		t.Fatalf("Aggregate with Having returned %d results, %v, want 2", len(expensive), err)
	}

	months, err := Aggregate[Payment, PaymentsByMonth](ctx, crud, AggregateOptions{
		Filters: sqlfilters.Filters{"PaidAt": {Op: sqlfilters.OpNotEqual, Val: nil}},
		Order:   []string{"Month", "asc"},
	})
	if err != nil || len(months) != 2 {
		t.Fatalf("Aggregate by month returned %d results, %v, want 2", len(months), err)
	}
	if !months[0].Month.Time.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || months[0].Total.cents != 3550 ||
		!months[1].Month.Time.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) || months[1].Total.cents != 100 {
		t.Fatalf("Aggregate by month returned %+v and %+v", *months[0], *months[1])
	}

	refs, err := Aggregate[Payment, PaymentsByRef](ctx, crud, AggregateOptions{})
	if err != nil || len(refs) != len(payments) {
		t.Fatalf("Aggregate by UUID returned %d results, %v, want %d", len(refs), err, len(payments))
	}
	found := map[uuid.UUID]bool{}
	for _, ref := range refs {
		found[ref.Ref] = ref.Payments == 1
	}
	for _, payment := range payments {
		if !found[payment.Ref] {
			t.Fatalf("Aggregate by UUID returned %v, without one payment of %s", found, payment.Ref)
		}
	}
}

func TestValuerAggregate(t *testing.T)        { testValuerAggregate(t, testCRUD) }
func TestSQLite_ValuerAggregate(t *testing.T) { testValuerAggregate(t, testCRUDSQLite) }

func TestCoalesce(t *testing.T) {
	for _, test := range []struct {
		value   interface{}
		dialect string
		want    string
	}{
		{Money{}, DialectPostgres, "COALESCE(x,'0.00')"},
		{&Money{}, DialectPostgres, "x"},
		{sql.NullInt64{}, DialectPostgres, "x"},
		{Level(0), DialectPostgres, "COALESCE(x,0)"},
		{Flag(false), DialectPostgres, "COALESCE(x,false)"},
		{Flag(false), DialectSQLite, "COALESCE(x,0)"},
		{"", DialectSQLite, "COALESCE(x,'')"},
		{[]byte{}, DialectSQLite, "COALESCE(x,X'')"},
		{time.Time{}, DialectPostgres, "x"},
	} {
		if got := coalesce(dialect.For(test.dialect), "x", reflect.TypeOf(test.value)); got != test.want {
			t.Fatalf("coalesce of %T in %s returned %s, want %s", test.value, test.dialect, got, test.want)
		}
	}
}
