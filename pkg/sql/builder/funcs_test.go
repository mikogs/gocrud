package builder

import (
	"testing"
	"time"
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
