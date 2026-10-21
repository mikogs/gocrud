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

func TestOpVal_IsNull(t *testing.T) {
	tests := map[string]struct {
		opVal OpVal
		want  bool
	}{
		"nil with OpEqual":    {OpVal{Op: OpEqual, Val: nil}, true},
		"nil with OpNotEqual": {OpVal{Op: OpNotEqual, Val: nil}, true},
		"typed nil":           {OpVal{Op: OpEqual, Val: (*int)(nil)}, true},
		"nil with OpGreater":  {OpVal{Op: OpGreater, Val: nil}, false},
		"value":               {OpVal{Op: OpEqual, Val: 0}, false},
		"pointer to a value":  {OpVal{Op: OpEqual, Val: new(int)}, false},
		"empty string":        {OpVal{Op: OpEqual, Val: ""}, false},
	}
	for name, test := range tests {
		if got := test.opVal.IsNull(); got != test.want {
			t.Fatalf("%s: IsNull returned %v, want %v", name, got, test.want)
		}
	}
}

func TestSetObjFields_Nullable(t *testing.T) {
	obj := &struct {
		Age       *int64
		Nickname  *string
		BirthDate *time.Time
	}{}
	nickname := "ali"

	err := SetObjFields(obj, &Filters{
		"Age":       {Val: 30},
		"Nickname":  {Val: &nickname},
		"BirthDate": {Val: time.Date(1990, 5, 1, 0, 0, 0, 0, time.UTC)},
	})
	if err != nil || obj.Age == nil || *obj.Age != 30 || obj.Nickname == nil || *obj.Nickname != "ali" || obj.BirthDate == nil {
		t.Fatalf("SetObjFields returned %v and set %+v", err, *obj)
	}

	if err := SetObjFields(obj, &Filters{"Age": {Val: nil}, "Nickname": {Val: (*string)(nil)}}); err != nil || obj.Age != nil || obj.Nickname != nil {
		t.Fatalf("SetObjFields returned %v and set %+v, want NULL", err, *obj)
	}

	if err := SetObjFields(obj, &Filters{"Age": {Val: "30"}}); err == nil {
		t.Fatal("SetObjFields did not return an err for a value of an incompatible type")
	}
}

func TestOpVal_InValues(t *testing.T) {
	tests := map[string]struct {
		val    interface{}
		want   []interface{}
		wantOK bool
	}{
		"slice":              {[]uint64{1, 2}, []interface{}{uint64(1), uint64(2)}, true},
		"array":              {[2]string{"a", "b"}, []interface{}{"a", "b"}, true},
		"values of any type": {[]interface{}{1, "a"}, []interface{}{1, "a"}, true},
		"empty slice":        {[]int{}, []interface{}{}, true},
		"not a list":         {"a,b", nil, false},
		"nil":                {nil, nil, false},
	}
	for name, test := range tests {
		got, ok := OpVal{Op: OpIn, Val: test.val}.InValues()
		if ok != test.wantOK || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s: InValues returned %v, %v, want %v, %v", name, got, ok, test.want, test.wantOK)
		}
	}
}
