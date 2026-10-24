package sqlfilters

import (
	"reflect"
	"testing"
)

func TestConditions(t *testing.T) {
	f := And(
		Filters{"Status": {Op: OpEqual, Val: "paid"}, "Age": {Op: OpGreater, Val: 30}},
		Or(
			&Filters{"Name": {Op: OpLike, Val: "Jo%"}},
			Raw(".Age < ?", 100),
		),
		Filters{"Age": {Op: OpLower, Val: 60}},
	)

	want := []Condition{
		{"Age", OpVal{Op: OpGreater, Val: 30}},
		{"Status", OpVal{Op: OpEqual, Val: "paid"}},
		{"Name", OpVal{Op: OpLike, Val: "Jo%"}},
		{"Age", OpVal{Op: OpLower, Val: 60}},
	}
	if got := Conditions(f); !reflect.DeepEqual(got, want) {
		t.Fatalf("Conditions returned %v, want %v", got, want)
	}
}

func TestFiltersInterfaces_Tree(t *testing.T) {
	f := And(
		Filters{
			"Status":    {Op: OpIn, Val: []string{"new", "paid"}},
			"ShippedAt": {Op: OpEqual, Val: nil},
			"Age":       {Op: OpGreater, Val: 30},
		},
		Or(
			Raw(".ID IN (?) OR .Name = ? OR .Code IN (?)", []uint64{1, 2}, nil, [2]string{"a", "b"}),
			Filters{"Price": {Op: OpLower, Val: 9.99}},
		),
		Or(),
		Raw(".Age < ?", 100),
	)

	want := []any{30, "new", "paid", uint64(1), uint64(2), nil, "a", "b", 9.99, 100}
	if got := FiltersInterfaces(f); !reflect.DeepEqual(got, want) {
		t.Fatalf("FiltersInterfaces returned %v, want %v", got, want)
	}
}

func TestFiltersInterfaces_NoFilters(t *testing.T) {
	var nilFilters *Filters
	for name, f := range map[string]Filter{
		"nil":            nil,
		"nil *Filters":   nilFilters,
		"empty And":      And(),
		"empty Or":       Or(),
		"empty Filters":  Filters{},
		"raw w/o values": Raw(".Age > 0"),
		"null condition": Filters{"Age": {Op: OpNotEqual, Val: nil}},
	} {
		if got := FiltersInterfaces(f); len(got) != 0 {
			t.Fatalf("%s: FiltersInterfaces returned %v, want none", name, got)
		}
	}
}

func TestAndOr(t *testing.T) {
	a, b := Filters{"Age": {Op: OpGreater, Val: 30}}, Raw(".Age < ?", 60)

	if got, want := And(a, b), (Group{Filters: []Filter{a, b}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("And returned %+v, want %+v", got, want)
	}
	if got, want := Or(a, b), (Group{Or: true, Filters: []Filter{a, b}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Or returned %+v, want %+v", got, want)
	}
}
