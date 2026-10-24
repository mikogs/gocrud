package gocrud

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type Payment struct {
	ID     uint64
	Amount Money  `crud:"type:numeric(12,2)"`
	Refund *Money `crud:"type:numeric(12,2)"`
	Ref    uuid.UUID
	Note   sql.NullString
	Count  sql.NullInt64
	PaidAt sql.NullTime
	Data   []byte
}

// JSONNote has a string stored as JSON, which cannot be compared with the strings of other columns.
type JSONNote struct {
	ID   uint64
	Note string `crud:"json"`
}

func testValuerFilters(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[Payment](ctx, crud)
	if err := CreateTable[Payment](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	refs := []uuid.UUID{uuid.MustParse("0190c4a2-7b3e-7cc1-9a3f-1d2e3f405161"), uuid.New(), uuid.New()}
	paidAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	refund := Money{cents: 500}
	for _, payment := range []*Payment{
		{Amount: Money{cents: 1000}, Ref: refs[0], Note: sql.NullString{String: "a", Valid: true}, Count: sql.NullInt64{Int64: 1, Valid: true},
			PaidAt: sql.NullTime{Time: paidAt, Valid: true}, Data: []byte{1}},
		{Amount: Money{cents: 2550}, Refund: &refund, Ref: refs[1], PaidAt: sql.NullTime{Time: paidAt.Add(time.Hour), Valid: true}, Data: []byte{2}},
		{Amount: Money{cents: 725}, Ref: refs[2], Note: sql.NullString{String: "b", Valid: true}, Count: sql.NullInt64{Int64: 3, Valid: true}},
	} {
		if err := crud.Save(ctx, payment, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	ids := func(t *testing.T, options GetOptions) []uint64 {
		t.Helper()
		options.Order = []string{"ID", "asc"}
		payments, err := Get[Payment](ctx, crud, options)
		if err != nil {
			t.Fatalf("Get failed: %s", err.Error())
		}
		var list []uint64
		for _, payment := range payments {
			list = append(list, payment.ID)
		}
		return list
	}

	cet := time.FixedZone("CET", 3600)
	for name, test := range map[string]struct {
		filters sqlfilters.Filters
		want    []uint64
	}{
		"value of the type":      {sqlfilters.Filters{"Amount": {Op: sqlfilters.OpGreater, Val: Money{cents: 1000}}}, []uint64{2}},
		"string on a decimal":    {sqlfilters.Filters{"Amount": {Op: sqlfilters.OpGreater, Val: "9.99"}}, []uint64{1, 2}},
		"float on a decimal":     {sqlfilters.Filters{"Amount": {Op: sqlfilters.OpLower, Val: 10.0}}, []uint64{3}},
		"in":                     {sqlfilters.Filters{"Amount": {Op: sqlfilters.OpIn, Val: []any{"10.00", Money{cents: 725}}}}, []uint64{1, 3}},
		"pointer field":          {sqlfilters.Filters{"Refund": {Op: sqlfilters.OpEqual, Val: "5.00"}}, []uint64{2}},
		"NULL of a pointer":      {sqlfilters.Filters{"Refund": {Op: sqlfilters.OpEqual, Val: nil}}, []uint64{1, 3}},
		"UUID in upper case":     {sqlfilters.Filters{"Ref": {Op: sqlfilters.OpEqual, Val: strings.ToUpper(refs[1].String())}}, []uint64{2}},
		"like":                   {sqlfilters.Filters{"Note": {Op: sqlfilters.OpLike, Val: "%b"}}, []uint64{3}},
		"time in another zone":   {sqlfilters.Filters{"PaidAt": {Op: sqlfilters.OpGreater, Val: paidAt.In(cet)}}, []uint64{2}},
		"NULL":                   {sqlfilters.Filters{"Note": {Op: sqlfilters.OpEqual, Val: nil}}, []uint64{2}},
		"not NULL":               {sqlfilters.Filters{"PaidAt": {Op: sqlfilters.OpNotEqual, Val: nil}}, []uint64{1, 2}},
		"string on a NullString": {sqlfilters.Filters{"Note": {Op: sqlfilters.OpEqual, Val: "a"}}, []uint64{1}},
		"int on a NullInt64":     {sqlfilters.Filters{"Count": {Op: sqlfilters.OpGreaterOrEqual, Val: 1}}, []uint64{1, 3}},
		"bytes":                  {sqlfilters.Filters{"Data": {Op: sqlfilters.OpEqual, Val: []byte{2}}}, []uint64{2}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ids(t, GetOptions{Filters: test.filters}); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Get returned %v, want %v", got, test.want)
			}
		})
	}

	for name, test := range map[string]struct {
		filters sqlfilters.Filters
		want    []uint64
	}{
		"decimal":       {sqlfilters.Filters{"Amount": {Op: sqlfilters.OpGreater, Val: "10"}}, []uint64{2}},
		"UUID":          {sqlfilters.Filters{"Ref": {Op: sqlfilters.OpIn, Val: strings.ToUpper(refs[0].String() + "," + refs[2].String())}}, []uint64{1, 3}},
		"time":          {sqlfilters.Filters{"PaidAt": {Op: sqlfilters.OpLowerOrEqual, Val: "2026-01-02T04:04:05+01:00"}}, []uint64{1}},
		"NullInt64":     {sqlfilters.Filters{"Count": {Op: sqlfilters.OpEqual, Val: "3"}}, []uint64{3}},
		"empty is NULL": {sqlfilters.Filters{"Note": {Op: sqlfilters.OpEqual, Val: ""}}, []uint64{2}},
		"empty pointer": {sqlfilters.Filters{"Refund": {Op: sqlfilters.OpEqual, Val: ""}}, []uint64{1, 3}},
		"NullString":    {sqlfilters.Filters{"Note": {Op: sqlfilters.OpEqual, Val: "b"}}, []uint64{3}},
	} {
		t.Run("from strings: "+name, func(t *testing.T) {
			if got := ids(t, GetOptions{Filters: test.filters, ConvertFiltersFromString: true}); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Get returned %v, want %v", got, test.want)
			}
		})
	}

	for field, options := range map[string]GetOptions{
		"Amount": {Filters: sqlfilters.Filters{"Amount": {Op: sqlfilters.OpEqual, Val: "ten"}}},
		"Ref":    {Filters: sqlfilters.Filters{"Ref": {Op: sqlfilters.OpEqual, Val: "not a UUID"}}},
		"Data":   {Filters: sqlfilters.Filters{"Data": {Op: sqlfilters.OpLike, Val: "x%"}}},
		"Count":  {Filters: sqlfilters.Filters{"Count": {Op: sqlfilters.OpEqual, Val: "x"}}, ConvertFiltersFromString: true},
	} {
		_, err := Get[Payment](ctx, crud, options)
		fatalIfNotViolations(t, err, map[string]string{field: ViolationType})
	}

	_, err := Get[Payment](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"Note": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&JSONItem{}, "Labels", nil)}}})
	if !errors.Is(err, ErrSubquery) {
		t.Fatalf("Get with a subquery of a JSON field returned err %v, want ErrSubquery", err)
	}
	_, err = Get[JSONNote](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"Note": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&Payment{}, "Note", nil)}}})
	if !errors.Is(err, ErrSubquery) {
		t.Fatalf("Get with a subquery on a JSON field returned err %v, want ErrSubquery", err)
	}

	err = UpdateMultiple[Payment](ctx, crud, map[string]any{"Amount": "99.99", "Note": nil, "Ref": strings.ToUpper(refs[0].String())},
		UpdateMultipleOptions{Filters: sqlfilters.Filters{"ID": {Op: sqlfilters.OpEqual, Val: 1}}})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	err = UpdateMultiple[Payment](ctx, crud, map[string]any{"Count": "", "Refund": "1.50"},
		UpdateMultipleOptions{Filters: sqlfilters.Filters{"ID": {Op: sqlfilters.OpEqual, Val: 3}}, ConvertValuesFromString: true})
	if err != nil {
		t.Fatalf("UpdateMultiple from strings failed: %s", err.Error())
	}
	payments, err := Get[Payment](ctx, crud, GetOptions{Order: []string{"ID", "asc"}})
	if err != nil || len(payments) != 3 {
		t.Fatalf("Get returned %d payments, %v, want 3", len(payments), err)
	}
	if first := payments[0]; first.Amount.cents != 9999 || first.Note.Valid || first.Ref != refs[0] {
		t.Fatalf("UpdateMultiple set %+v", first)
	}
	if third := payments[2]; third.Count.Valid || third.Refund == nil || third.Refund.cents != 150 {
		t.Fatalf("UpdateMultiple from strings set %+v", third)
	}
	if got := ids(t, GetOptions{Filters: sqlfilters.Filters{"Ref": {Op: sqlfilters.OpEqual, Val: refs[0].String()}}}); !reflect.DeepEqual(got, []uint64{1}) {
		t.Fatalf("Get by the UUID set by UpdateMultiple returned %v, want [1]", got)
	}

	err = UpdateMultiple[Payment](ctx, crud, map[string]any{"Amount": "ten"}, UpdateMultipleOptions{})
	fatalIfNotViolations(t, err, map[string]string{"Amount": ViolationType})
}

func TestValuerFilters(t *testing.T)        { testValuerFilters(t, testCRUD) }
func TestSQLite_ValuerFilters(t *testing.T) { testValuerFilters(t, testCRUDSQLite) }

func TestValueKind(t *testing.T) {
	for _, test := range []struct {
		value any
		want  string
	}{
		{int8(0), "number"},
		{uint64(0), "number"},
		{float32(0), "number"},
		{Level(0), "number"},
		{sql.NullInt64{}, "number"},
		{Money{}, "string"},
		{uuid.UUID{}, "string"},
		{time.Time{}, "time.Time"},
		{sql.NullTime{}, "time.Time"},
		{Flag(false), "bool"},
		{[]byte{}, "[]byte"},
		{map[string]int{}, ""},
	} {
		if got := valueKind(reflect.TypeOf(test.value)); got != test.want {
			t.Fatalf("valueKind of %T returned %q, want %q", test.value, got, test.want)
		}
	}
}
