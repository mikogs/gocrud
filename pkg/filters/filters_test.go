package filters

import (
	"reflect"
	"testing"
	"time"
)

type setObjFieldsTestStruct struct {
	Name     string
	Age      int
	Price    float64
	Tags     []string
	internal string
}

func TestFilters_Add(t *testing.T) {
	f := Filters{}
	f.Add("Name", OpVal{Op: OpEqual, Val: "John"})
	f.Add("Name", OpVal{Op: OpLike, Val: "Jo%"})

	want := Filters{"Name": {Op: OpLike, Val: "Jo%"}}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("Add returned %v, want %v", f, want)
	}
}

func TestFiltersInterfaces(t *testing.T) {
	f := &Filters{
		"Name":  {Op: OpEqual, Val: "John"},
		"Age":   {Op: OpGreater, Val: 30},
		"Price": {Op: OpLower, Val: 9.99},
		Raw: {Op: OpAND, Val: []interface{}{
			".Age < ? AND .ID IN (?) AND .Name IN (?)",
			100,
			[]int{1, 2},
			[]string{"a", "b"},
		}},
	}

	want := []interface{}{30, "John", 9.99, 100, 1, 2, "a", "b"}
	if got := FiltersInterfaces(f); !reflect.DeepEqual(got, want) {
		t.Fatalf("FiltersInterfaces returned %v, want %v", got, want)
	}
}

func TestFiltersInterfaces_Empty(t *testing.T) {
	if got := FiltersInterfaces(nil); len(got) != 0 {
		t.Fatalf("FiltersInterfaces returned %v for nil filters, want none", got)
	}
	if got := FiltersInterfaces(&Filters{}); len(got) != 0 {
		t.Fatalf("FiltersInterfaces returned %v for empty filters, want none", got)
	}
}

func TestSetObjFields(t *testing.T) {
	obj := &setObjFieldsTestStruct{}
	err := SetObjFields(obj, &Filters{
		"Name":     {Val: "John"},
		"Age":      {Val: int64(37)},
		"Price":    {Val: 10},
		"Tags":     {Val: []string{"a"}},
		"internal": {Val: "x"},
		"Unknown":  {Val: "x"},
		Raw:        {Val: []interface{}{".Age > ?", 1}},
	})
	if err != nil {
		t.Fatalf("SetObjFields failed with an err: %s", err.Error())
	}

	want := setObjFieldsTestStruct{Name: "John", Age: 37, Price: 10}
	if !reflect.DeepEqual(*obj, want) {
		t.Fatalf("SetObjFields set %+v, want %+v", *obj, want)
	}
}

func TestSetObjFields_IncompatibleType(t *testing.T) {
	obj := &setObjFieldsTestStruct{}
	err := SetObjFields(obj, &Filters{
		"Age":  {Val: "notanint"},
		"Name": {Val: "John"},
	})
	want := "cannot set field Age (int) with value of type string"
	if err == nil || err.Error() != want {
		t.Fatalf("SetObjFields returned err %v, want %q", err, want)
	}
	if obj.Name != "John" {
		t.Fatal("SetObjFields did not set the remaining fields")
	}
}

func TestSetObjFields_InvalidObject(t *testing.T) {
	var nilObj *setObjFieldsTestStruct
	for name, obj := range map[string]interface{}{"nil pointer": nilObj, "not a struct": "string"} {
		if err := SetObjFields(obj, &Filters{"Name": {Val: "John"}}); err == nil {
			t.Fatalf("SetObjFields did not return an err for %s", name)
		}
	}
}

func TestSetObjFields_NilValue(t *testing.T) {
	obj := &setObjFieldsTestStruct{Name: "John"}
	err := SetObjFields(obj, &Filters{"Name": {Val: nil}})
	if err == nil {
		t.Fatal("SetObjFields did not return an err for a nil value")
	}
	if obj.Name != "John" {
		t.Fatalf("SetObjFields changed the field to %q", obj.Name)
	}
}

func TestSetObjFields_IntIntoString(t *testing.T) {
	obj := &setObjFieldsTestStruct{}
	err := SetObjFields(obj, &Filters{"Name": {Val: 65}})
	if err == nil {
		t.Fatalf("SetObjFields did not return an err, set field to %q", obj.Name)
	}
}

func TestFiltersInterfaces_InvalidRaw(t *testing.T) {
	for _, rawValue := range []interface{}{nil, ".Age > ?", []string{".Age > ?", "1"}, []interface{}{}} {
		f := &Filters{
			"Name": {Op: OpEqual, Val: "John"},
			Raw:    {Op: OpAND, Val: rawValue},
		}
		if got := FiltersInterfaces(f); !reflect.DeepEqual(got, []interface{}{"John"}) {
			t.Fatalf("FiltersInterfaces returned %v for raw filter %#v, want [John]", got, rawValue)
		}
	}
}

func TestFiltersInterfaces_RawNilValue(t *testing.T) {
	f := &Filters{Raw: {Op: OpAND, Val: []interface{}{".Age = ? OR .Name = ?", nil, "John"}}}

	want := []interface{}{nil, "John"}
	if got := FiltersInterfaces(f); !reflect.DeepEqual(got, want) {
		t.Fatalf("FiltersInterfaces returned %v, want %v", got, want)
	}
}

func TestSetObjFields_Time(t *testing.T) {
	obj := &struct {
		StartsAt time.Time
	}{}

	startsAt := time.Date(2024, 5, 1, 10, 30, 0, 0, time.UTC)
	if err := SetObjFields(obj, &Filters{"StartsAt": {Val: startsAt}}); err != nil || !obj.StartsAt.Equal(startsAt) {
		t.Fatalf("SetObjFields returned %v and set %v, want %v", err, obj.StartsAt, startsAt)
	}

	if err := SetObjFields(obj, &Filters{"StartsAt": {Val: "2024-05-01"}}); err == nil {
		t.Fatal("SetObjFields did not return an err for a string value")
	}
}
