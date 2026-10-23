package gocrud

import (
	"context"
	"math"
	"testing"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type NumberRanges struct {
	ID     uint64
	Age    uint8 `validate:"max=120"`
	Small  int8
	Count  uint32
	Port   uint16
	Amount int64 `validate:"min=1"`
}

func testNumberFilters(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = crud.DropTable(ctx, &NumberRanges{})
	if err := crud.CreateTable(ctx, &NumberRanges{}); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	for _, test := range []struct {
		filter  sqlfilters.Filters
		convert bool
		want    map[string]string
	}{
		{sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 300}}, false, map[string]string{"Age": ViolationType}},
		{sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: -1}}, false, map[string]string{"Age": ViolationType}},
		{sqlfilters.Filters{"Age": {Op: sqlfilters.OpIn, Val: []int{1, 300}}}, false, map[string]string{"Age": ViolationType}},
		{sqlfilters.Filters{"Amount": {Op: sqlfilters.OpEqual, Val: 0.5}}, false, map[string]string{"Amount": ViolationType}},
		{sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 121}}, false, map[string]string{"Age": "max"}},
		{sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: "300"}}, true, map[string]string{"Age": ViolationType}},
		{sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: "256"}}, true, map[string]string{"Age": ViolationType}},
		{sqlfilters.Filters{"Small": {Op: sqlfilters.OpEqual, Val: "200"}}, true, map[string]string{"Small": ViolationType}},
		{sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: "121"}}, true, map[string]string{"Age": "max"}},
	} {
		_, err := Get[NumberRanges](ctx, crud, GetOptions{Filters: test.filter, ConvertFiltersFromString: test.convert})
		fatalIfNotViolations(t, err, test.want)
	}

	if _, err := Get[NumberRanges](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 120}, "Amount": {Op: sqlfilters.OpEqual, Val: 2.0}}}); err != nil {
		t.Fatalf("Get with numbers that fit failed: %s", err.Error())
	}
}

func testNumberRanges(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = crud.DropTable(ctx, &NumberRanges{})
	if err := crud.CreateTable(ctx, &NumberRanges{}); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	obj := &NumberRanges{Age: 120, Small: math.MinInt8, Count: math.MaxUint32, Port: math.MaxUint16, Amount: math.MaxInt64}
	if err := crud.Save(ctx, obj, SaveOptions{}); err != nil {
		t.Fatalf("Save of the largest values failed: %s", err.Error())
	}

	loaded, err := Load[NumberRanges](ctx, crud, obj.ID)
	if err != nil || *loaded != *obj {
		t.Fatalf("Load returned %+v, %v, want %+v", loaded, err, *obj)
	}
}

func TestNumberFilters(t *testing.T)        { testNumberFilters(t, testCRUD) }
func TestSQLite_NumberFilters(t *testing.T) { testNumberFilters(t, testCRUDSQLite) }

func TestNumberRanges(t *testing.T)        { testNumberRanges(t, testCRUD) }
func TestSQLite_NumberRanges(t *testing.T) { testNumberRanges(t, testCRUDSQLite) }
