package sqlbuilder

import (
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type structFieldValueFromString struct {
	Int64Field uint64
	TimeField  time.Time
}

func TestStructFieldValueFromString(t *testing.T) {
	testObj := &structFieldValueFromString{}

	ok, value := StructFieldValueFromString(testObj, "Int64Field", "0")
	if !ok {
		t.Fatal("Failed to parse string value")
	}

	if value.(uint64) != 0 {
		t.Fatal("Parsed value is invalid")
	}
}

func TestSetStructFieldValueFromString(t *testing.T) {
	testObj := &structFieldValueFromString{}

	ok, value := SetStructFieldValueFromString(testObj, "Int64Field", "8")
	if !ok {
		t.Fatal("Failed to parse string value")
	}

	if value.(uint64) != 8 {
		t.Fatal("Parsed value is invalid")
	}
}

func TestTimeFromString(t *testing.T) {
	tests := map[string]time.Time{
		"2024-05-01T10:30:15.5+02:00": time.Date(2024, 5, 1, 8, 30, 15, 500000000, time.UTC),
		"2024-05-01T10:30:15Z":        time.Date(2024, 5, 1, 10, 30, 15, 0, time.UTC),
		"2024-05-01T10:30:15":         time.Date(2024, 5, 1, 10, 30, 15, 0, time.UTC),
		"2024-05-01 10:30:15":         time.Date(2024, 5, 1, 10, 30, 15, 0, time.UTC),
		"2024-05-01T10:30":            time.Date(2024, 5, 1, 10, 30, 0, 0, time.UTC),
		"2024-05-01":                  time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
	}
	for value, want := range tests {
		got, ok := TimeFromString(value)
		if !ok || !got.Equal(want) {
			t.Fatalf("TimeFromString(%q) returned %v, %v, want %v", value, got, ok, want)
		}
	}

	for _, value := range []string{"", "yesterday", "01/05/2024", "2024-13-01", "1714559415"} {
		if _, ok := TimeFromString(value); ok {
			t.Fatalf("TimeFromString(%q) did not fail", value)
		}
	}
}

func TestSetStructFieldValueFromString_Time(t *testing.T) {
	testObj := &structFieldValueFromString{}

	ok, value := SetStructFieldValueFromString(testObj, "TimeField", "2024-05-01T10:30")
	want := time.Date(2024, 5, 1, 10, 30, 0, 0, time.UTC)
	if !ok || !value.(time.Time).Equal(want) || !testObj.TimeField.Equal(want) {
		t.Fatalf("SetStructFieldValueFromString returned %v, %v and set %v, want %v", ok, value, testObj.TimeField, want)
	}

	ok, _ = SetStructFieldValueFromString(testObj, "TimeField", "not a time")
	if ok || !testObj.TimeField.Equal(want) {
		t.Fatal("SetStructFieldValueFromString did not fail for an invalid time")
	}
}

func TestSetStructFieldValueFromString_Nullable(t *testing.T) {
	testObj := &struct {
		Age       *int64
		Nickname  *string
		BirthDate *time.Time
	}{}

	ok, value := SetStructFieldValueFromString(testObj, "Age", "30")
	if !ok || value.(int64) != 30 || testObj.Age == nil || *testObj.Age != 30 {
		t.Fatalf("SetStructFieldValueFromString returned %v, %v and set %v, want 30", ok, value, testObj.Age)
	}

	ok, value = SetStructFieldValueFromString(testObj, "BirthDate", "1990-05-01")
	if !ok || testObj.BirthDate == nil || !testObj.BirthDate.Equal(time.Date(1990, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("SetStructFieldValueFromString returned %v, %v and set %v", ok, value, testObj.BirthDate)
	}

	for _, name := range []string{"Age", "Nickname", "BirthDate"} {
		ok, value = SetStructFieldValueFromString(testObj, name, "")
		if !ok || value != nil {
			t.Fatalf("SetStructFieldValueFromString returned %v, %v for an empty %s, want NULL", ok, value, name)
		}
	}
	if testObj.Age != nil || testObj.Nickname != nil || testObj.BirthDate != nil {
		t.Fatalf("SetStructFieldValueFromString did not set NULL: %+v", *testObj)
	}

	if ok, _ = SetStructFieldValueFromString(testObj, "Age", "thirty"); ok || testObj.Age != nil {
		t.Fatal("SetStructFieldValueFromString did not fail for an invalid value")
	}
}

type numbersFromString struct {
	Int8    int8
	Uint8   uint8
	Uint16  uint16
	Float32 float32
}

func TestStructFieldValueFromString_NumbersThatDoNotFit(t *testing.T) {
	tests := []struct {
		field string
		value string
		fits  bool
	}{
		{"Uint8", "255", true},
		{"Uint8", "256", false},
		{"Uint8", "-1", false},
		{"Int8", "-128", true},
		{"Int8", "200", false},
		{"Uint16", "65535", true},
		{"Uint16", "65536", false},
		{"Float32", "340000000000000000000000000000000000000.5", true},
		{"Float32", "1000000000000000000000000000000000000000.5", false},
	}

	for _, test := range tests {
		obj := &numbersFromString{}
		ok, _ := SetStructFieldValueFromString(obj, test.field, test.value)
		if ok != test.fits {
			t.Fatalf("SetStructFieldValueFromString of %q in %s returned %v, want %v", test.value, test.field, ok, test.fits)
		}
	}

	obj := &numbersFromString{}
	if ok, _ := SetStructFieldValueFromString(obj, "Uint8", "255"); !ok || obj.Uint8 != 255 {
		t.Fatalf("SetStructFieldValueFromString set %d, want 255", obj.Uint8)
	}
}

// hexBytes is written as bytes, and its text is hex
type hexBytes []byte

func (h hexBytes) Value() (driver.Value, error) { return []byte(h), nil }
func (h *hexBytes) Scan(src any) error          { *h = src.([]byte); return nil }
func (h *hexBytes) UnmarshalText(text []byte) (err error) {
	*h, err = hex.DecodeString(string(text))
	return err
}

type valuersFromString struct {
	Hex    hexBytes
	Ref    uuid.UUID
	Note   sql.NullString
	Count  sql.NullInt64
	At     sql.NullTime
	Amount testMoney
	Opt    *sql.NullInt64
}

func TestSetStructFieldValueFromString_Valuers(t *testing.T) {
	ref := uuid.MustParse("0190c4a2-7b3e-7cc1-9a3f-1d2e3f405162")
	tests := []struct {
		field string
		value string
		want  any
	}{
		{"Ref", strings.ToUpper(ref.String()), ref},
		{"Hex", "ab01", hexBytes{0xab, 1}},
		{"Note", "a", sql.NullString{String: "a", Valid: true}},
		{"Note", "", nil},
		{"Count", "-3", sql.NullInt64{Int64: -3, Valid: true}},
		{"At", "2024-05-01T10:30:00+02:00", sql.NullTime{Time: time.Date(2024, 5, 1, 8, 30, 0, 0, time.UTC), Valid: true}},
		{"Amount", "1.50", testMoney{}},
		{"Opt", "", nil},
		{"Opt", "7", sql.NullInt64{Int64: 7, Valid: true}},
	}

	for _, test := range tests {
		obj := &valuersFromString{Note: sql.NullString{String: "old", Valid: true}, Opt: &sql.NullInt64{Int64: 1, Valid: true}}
		ok, value := SetStructFieldValueFromString(obj, test.field, test.value)
		if at, isTime := value.(sql.NullTime); isTime {
			value = sql.NullTime{Time: at.Time.UTC(), Valid: at.Valid}
		}
		if !ok || !reflect.DeepEqual(value, test.want) {
			t.Fatalf("SetStructFieldValueFromString of %q in %s returned %v, %+v, want %+v", test.value, test.field, ok, value, test.want)
		}
		field := reflect.ValueOf(obj).Elem().FieldByName(test.field)
		if test.want == nil && !field.IsZero() {
			t.Fatalf("SetStructFieldValueFromString set %s to %+v, want NULL", test.field, field.Interface())
		}
	}

	for field, value := range map[string]string{"Ref": "x", "Count": "1.5", "At": "yesterday", "Hex": "x"} {
		if ok, _ := StructFieldValueFromString(&valuersFromString{}, field, value); ok {
			t.Fatalf("StructFieldValueFromString of %q in %s succeeded", value, field)
		}
	}
}
