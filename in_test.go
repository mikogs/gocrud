package gocrud

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type Shipment struct {
	ID       uint64
	Status   string `validate:"oneof=new paid shipped delivered"`
	Weight   int64  `validate:"min=0"`
	Carrier  *string
	PickedUp time.Time
}

func createShipments(t *testing.T, crud *CRUD) []*Shipment {
	t.Helper()
	ctx := context.Background()

	_ = DropTable[Shipment](ctx, crud)
	if err := CreateTable[Shipment](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	dhl, ups := "dhl", "ups"
	tokyo := mustLoadLocation(t, "Asia/Tokyo")
	shipments := []*Shipment{
		{Status: "new", Weight: 1, PickedUp: time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)},
		{Status: "paid", Weight: 2, Carrier: &dhl, PickedUp: time.Date(2024, 5, 1, 20, 0, 0, 0, tokyo)},
		{Status: "shipped", Weight: 3, Carrier: &ups, PickedUp: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)},
		{Status: "delivered", Weight: 4, Carrier: &dhl, PickedUp: time.Date(2024, 5, 2, 10, 0, 0, 0, time.UTC)},
	}
	for _, shipment := range shipments {
		if err := crud.Save(ctx, shipment, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	return shipments
}

func fatalIfNotShipments(t *testing.T, crud *CRUD, options GetOptions, want []uint64) {
	t.Helper()

	if options.Order == nil {
		options.Order = []string{"ID", "asc"}
	}
	shipments, err := Get[Shipment](context.Background(), crud, options)
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}

	got := make([]uint64, len(shipments))
	for i, shipment := range shipments {
		got[i] = shipment.ID
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get returned shipments %v, want %v", got, want)
	}

	cnt, err := GetCount[Shipment](context.Background(), crud, GetCountOptions{
		Filters:                  options.Filters,
		ConvertFiltersFromString: options.ConvertFiltersFromString,
	})
	if err != nil || cnt != uint64(len(want)) {
		t.Fatalf("GetCount returned %d, %v, want %d", cnt, err, len(want))
	}
}

func testInFilters(t *testing.T, crud *CRUD) {
	createShipments(t, crud)
	tokyo := mustLoadLocation(t, "Asia/Tokyo")

	tests := map[string]struct {
		filters sqlfilters.Filter
		want    []uint64
	}{
		"strings":            {sqlfilters.Filters{"Status": {Op: sqlfilters.OpIn, Val: []string{"paid", "shipped"}}}, []uint64{2, 3}},
		"IDs":                {sqlfilters.Filters{"ID": {Op: sqlfilters.OpIn, Val: []uint64{1, 4, 99}}}, []uint64{1, 4}},
		"array":              {sqlfilters.Filters{"Weight": {Op: sqlfilters.OpIn, Val: [2]int64{2, 4}}}, []uint64{2, 4}},
		"values of any type": {sqlfilters.Filters{"Weight": {Op: sqlfilters.OpIn, Val: []any{1, int64(3)}}}, []uint64{1, 3}},
		"one value":          {sqlfilters.Filters{"Status": {Op: sqlfilters.OpIn, Val: []string{"new"}}}, []uint64{1}},
		"empty list":         {sqlfilters.Filters{"Status": {Op: sqlfilters.OpIn, Val: []string{}}}, []uint64{}},
		"nullable column":    {sqlfilters.Filters{"Carrier": {Op: sqlfilters.OpIn, Val: []string{"dhl"}}}, []uint64{2, 4}},
		"times in other time zones": {sqlfilters.Filters{"PickedUp": {Op: sqlfilters.OpIn, Val: []time.Time{
			time.Date(2024, 5, 1, 19, 0, 0, 0, tokyo),
			time.Date(2024, 5, 1, 11, 0, 0, 0, time.UTC),
		}}}, []uint64{1, 2}},
		"with other filters": {sqlfilters.And(sqlfilters.Filters{
			"Carrier": {Op: sqlfilters.OpNotEqual, Val: nil},
			"ID":      {Op: sqlfilters.OpIn, Val: []uint64{1, 2, 3}},
			"Status":  {Op: sqlfilters.OpIn, Val: []string{"new", "paid", "shipped"}},
			"Weight":  {Op: sqlfilters.OpGreater, Val: 2},
		}, sqlfilters.Raw(".Weight IN (?) OR .Status = ?", []int{3, 4}, "new")), []uint64{3}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fatalIfNotShipments(t, crud, GetOptions{Filters: test.filters}, test.want)
		})
	}
}

func testInFiltersFromStrings(t *testing.T, crud *CRUD) {
	createShipments(t, crud)

	tests := map[string]struct {
		value string
		field string
		want  []uint64
	}{
		"strings":    {"paid,delivered", "Status", []uint64{2, 4}},
		"numbers":    {"1,3", "Weight", []uint64{1, 3}},
		"times":      {"2024-05-01T10:00:00Z,2024-05-02T10:00", "PickedUp", []uint64{1, 4}},
		"empty list": {"", "Status", []uint64{}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fatalIfNotShipments(t, crud, GetOptions{
				Filters:                  &sqlfilters.Filters{test.field: {Op: sqlfilters.OpIn, Val: test.value}},
				ConvertFiltersFromString: true,
			}, test.want)
		})
	}

	_, err := Get[Shipment](context.Background(), crud, GetOptions{
		Filters:                  &sqlfilters.Filters{"Weight": {Op: sqlfilters.OpIn, Val: "1,heavy"}},
		ConvertFiltersFromString: true,
	})
	fatalIfNotViolations(t, err, map[string]string{"Weight": ViolationType})
}

func testInFiltersValidation(t *testing.T, crud *CRUD) {
	createShipments(t, crud)

	tests := map[string]struct {
		filter sqlfilters.OpVal
		want   string
	}{
		"value breaks a rule":   {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: []string{"paid", "lost"}}, "oneof"},
		"value of another type": {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: []any{"paid", 5}}, ViolationType},
		"nil value":             {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: []any{"paid", nil}}, ViolationType},
		"not a list":            {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: "paid"}, ViolationType},
		"nil list":              {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: nil}, ViolationType},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Get[Shipment](context.Background(), crud, GetOptions{
				Filters: &sqlfilters.Filters{"Status": test.filter},
			})
			fatalIfNotViolations(t, err, map[string]string{"Status": test.want})
		})
	}
}

func testInFiltersUpdateAndDelete(t *testing.T, crud *CRUD) {
	createShipments(t, crud)
	ctx := context.Background()

	err := UpdateMultiple[Shipment](ctx, crud, map[string]any{"Status": "shipped"}, UpdateMultipleOptions{
		Filters: &sqlfilters.Filters{"ID": {Op: sqlfilters.OpIn, Val: []uint64{1, 2}}},
	})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	fatalIfNotShipments(t, crud, GetOptions{
		Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "shipped"}},
	}, []uint64{1, 2, 3})

	_, err = DeleteMultiple[Shipment](ctx, crud, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpIn, Val: []string{"shipped", "delivered"}}},
	})
	if err != nil {
		t.Fatalf("DeleteMultiple failed: %s", err.Error())
	}
	fatalIfNotShipments(t, crud, GetOptions{}, []uint64{})
}

func TestIn_Filters(t *testing.T)        { testInFilters(t, testCRUD) }
func TestSQLite_In_Filters(t *testing.T) { testInFilters(t, testCRUDSQLite) }

func TestIn_FiltersFromStrings(t *testing.T)        { testInFiltersFromStrings(t, testCRUD) }
func TestSQLite_In_FiltersFromStrings(t *testing.T) { testInFiltersFromStrings(t, testCRUDSQLite) }

func TestIn_Validation(t *testing.T)        { testInFiltersValidation(t, testCRUD) }
func TestSQLite_In_Validation(t *testing.T) { testInFiltersValidation(t, testCRUDSQLite) }

func TestIn_UpdateAndDelete(t *testing.T)        { testInFiltersUpdateAndDelete(t, testCRUD) }
func TestSQLite_In_UpdateAndDelete(t *testing.T) { testInFiltersUpdateAndDelete(t, testCRUDSQLite) }
