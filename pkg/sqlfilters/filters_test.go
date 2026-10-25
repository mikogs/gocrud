package sqlfilters

import (
	"database/sql"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

type setObjFieldsTestStruct struct {
	Name     string
	Age      int
	Price    float64
	Tags     []string
	internal string //nolint:unused // an unexported field, which SetObjFields does not set
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

func TestSetObjFields(t *testing.T) {
	obj := &setObjFieldsTestStruct{}
	err := SetObjFields(obj, &Filters{
		"Name":     {Val: "John"},
		"Age":      {Val: int64(37)},
		"Price":    {Val: 10},
		"Tags":     {Val: []string{"a"}},
		"internal": {Val: "x"},
		"Unknown":  {Val: "x"},
	})
	if err != nil {
		t.Fatalf("SetObjFields failed with an err: %s", err.Error())
	}

	want := setObjFieldsTestStruct{Name: "John", Age: 37, Price: 10, Tags: []string{"a"}}
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
	for name, obj := range map[string]any{"nil pointer": nilObj, "not a struct": "string"} {
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
		val    any
		want   []any
		wantOK bool
	}{
		"slice":              {[]uint64{1, 2}, []any{uint64(1), uint64(2)}, true},
		"array":              {[2]string{"a", "b"}, []any{"a", "b"}, true},
		"values of any type": {[]any{1, "a"}, []any{1, "a"}, true},
		"empty slice":        {[]int{}, []any{}, true},
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

type numbers struct {
	Int8    int8
	Int64   int64
	Uint8   uint8
	Uint64  uint64
	Float32 float32
	Float64 float64
}

func TestSetObjFields_NumbersThatDoNotFit(t *testing.T) {
	tests := []struct {
		field string
		value any
		fits  bool
	}{
		{"Uint8", 255, true},
		{"Uint8", 300, false},
		{"Uint8", -1, false},
		{"Int8", int64(-128), true},
		{"Int8", int64(-129), false},
		{"Int8", uint8(200), false},
		{"Int64", uint64(math.MaxInt64), true},
		{"Int64", uint64(math.MaxUint64), false},
		{"Uint64", uint64(math.MaxUint64), true},
		{"Uint64", -1, false},
		{"Uint8", uint16(255), true},
		{"Uint8", uint16(300), false},
		{"Uint64", -1.0, false},
		{"Int64", 2.0, true},
		{"Int64", 1.5, false},
		{"Int64", 1e19, false},
		{"Int64", math.NaN(), false},
		{"Int64", math.Inf(1), false},
		{"Uint8", -1.0, false},
		{"Uint64", 1e20, false},
		{"Float32", 3.4e38, true},
		{"Float32", 1e39, false},
		{"Float64", int64(math.MaxInt64), true},
		{"Float32", uint64(math.MaxUint64), true},
	}

	for _, test := range tests {
		err := SetObjFields(&numbers{}, &Filters{test.field: {Val: test.value}})
		if (err == nil) != test.fits {
			t.Fatalf("SetObjFields of %v (%T) in %s returned %v, want the value to fit: %v", test.value, test.value, test.field, err, test.fits)
		}
	}
}

func TestSetObjFields_Valuers(t *testing.T) {
	type valuers struct {
		Ref   uuid.UUID
		Count sql.NullInt64
		Note  sql.NullString
		Opt   *sql.NullInt64
	}
	ref := uuid.MustParse("0190c4a2-7b3e-7cc1-9a3f-1d2e3f405162")

	obj := &valuers{Note: sql.NullString{String: "old", Valid: true}}
	err := SetObjFields(obj, &Filters{
		"Ref":   {Val: ref.String()},
		"Count": {Val: 7},
		"Note":  {Val: nil},
		"Opt":   {Val: int8(2)},
	})
	want := valuers{Ref: ref, Count: sql.NullInt64{Int64: 7, Valid: true}, Opt: &sql.NullInt64{Int64: 2, Valid: true}}
	if err != nil || !reflect.DeepEqual(*obj, want) {
		t.Fatalf("SetObjFields set %+v, %v, want %+v", *obj, err, want)
	}

	if err := SetObjFields(&valuers{}, &Filters{"Ref": {Val: "x"}}); err == nil {
		t.Fatal("SetObjFields set a UUID from x")
	}
	if err := SetObjFields(&valuers{}, &Filters{"Ref": {Val: nil}}); err == nil {
		t.Fatal("SetObjFields set a UUID, whose zero value is not NULL, to nil")
	}
}
