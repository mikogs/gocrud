package structinfo

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

var (
	timeType    = reflect.TypeOf(time.Time{})
	boolType    = reflect.TypeOf(false)
	scannerType = reflect.TypeOf((*sql.Scanner)(nil)).Elem()
	valuerType  = reflect.TypeOf((*driver.Valuer)(nil)).Elem()
)

// IsFieldKindSupported checks if a specific reflect kind of the field is supported by the Builder.
// It does not include time.Time, which is a struct. Use IsFieldTypeSupported to check a field type.
func IsFieldKindSupported(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	case reflect.String, reflect.Bool:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// IsFieldTypeSupported checks if a field of a specific type can be stored in the database: a basic type, time.Time,
// []byte, a Valuer type, or a pointer to one of them, which is stored in a nullable column.
func IsFieldTypeSupported(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return IsFieldKindSupported(t.Kind()) || IsTimeType(t) || IsBytesType(t) || IsValuerType(t)
}

// IsNullableType checks if a field of a specific type is stored in a nullable column: a pointer, or a Valuer type
// whose zero value is NULL, e.g. sql.NullString.
func IsNullableType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		return IsFieldTypeSupported(t.Elem())
	}
	if !IsValuerType(t) {
		return false
	}
	zero, err := ZeroValue(t)
	return err == nil && zero == nil
}

// BaseType returns the type that a pointer field points to, or t itself for a field that is not a pointer.
func BaseType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer && IsFieldTypeSupported(t.Elem()) {
		return t.Elem()
	}

	return t
}

// IsBytesType checks if t is []byte, or a named type of it, e.g. json.RawMessage.
func IsBytesType(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8
}

// IsValuerType checks if t is a type that database/sql reads and writes with its methods: *t is a sql.Scanner, and t
// or *t is a driver.Valuer, e.g. sql.NullString or a decimal type.
func IsValuerType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		return false
	}
	pointer := reflect.PointerTo(t)
	return pointer.Implements(scannerType) && (t.Implements(valuerType) || pointer.Implements(valuerType))
}

// ZeroValue returns the value that the zero value of the Valuer type t is written to the database as.
func ZeroValue(t reflect.Type) (value driver.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the Value method of the zero value of %s panics: %v", t, r)
		}
	}()

	zero := reflect.New(t)
	if valuer, ok := zero.Elem().Interface().(driver.Valuer); ok {
		return valuer.Value()
	}
	return zero.Interface().(driver.Valuer).Value()
}

// JSONZeroValue returns the JSON of the zero value of t, or an error when encoding/json cannot write it, e.g. for a
// struct with a func field.
func JSONZeroValue(t reflect.Type) (value string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the JSON of the zero value of %s panics: %v", t, r)
		}
	}()

	data, err := json.Marshal(reflect.New(t).Interface())
	return string(data), err
}

// ValuerValueType returns the type of the values that the Valuer type t is written as: the type of its zero value, or
// of the value of a struct of a value and Valid, such as sql.NullInt64 and sql.Null[T], whose zero value is NULL. It is
// nil when the type is not known.
func ValuerValueType(t reflect.Type) reflect.Type {
	zero, err := ZeroValue(t)
	switch {
	case err != nil:
		return nil
	case zero != nil:
		return reflect.TypeOf(zero)
	case t.Kind() == reflect.Struct && t.NumField() == 2 && t.Field(1).Name == "Valid" && t.Field(1).Type == boolType:
		return t.Field(0).Type
	default:
		return nil
	}
}

// ScanValue returns value as a value of the Valuer type t, scanned as it would be from the database, e.g. a decimal
// from a string, and false when t cannot scan it.
func ScanValue(t reflect.Type, value any) (reflect.Value, bool) {
	driverValue, err := driver.DefaultParameterConverter.ConvertValue(value)
	if err != nil {
		return reflect.Value{}, false
	}

	scanned := reflect.New(t)
	if err := scanned.Interface().(sql.Scanner).Scan(driverValue); err != nil {
		return reflect.Value{}, false
	}
	return scanned.Elem(), true
}

// ColumnGoType returns the Go type that the column of a field of type t is created for: the name of the kind of a
// basic type, so that e.g. type Level int gets the column of int, time.Time, []byte, or for a Valuer type, the type of
// the value that its zero value is written as. The zero value of a struct of a value and Valid, such as sql.NullInt64
// and sql.Null[T], is NULL, so it gets the column of the value. It is empty when the type is not known.
func ColumnGoType(t reflect.Type) string {
	t = BaseType(t)
	switch {
	case IsValuerType(t):
		valueType := ValuerValueType(t)
		if valueType == nil {
			return ""
		}
		return ColumnGoType(valueType)
	case IsTimeType(t):
		return "time.Time"
	case IsBytesType(t):
		return "[]byte"
	case IsFieldKindSupported(t.Kind()):
		return t.Kind().String()
	default:
		return ""
	}
}

// IsNamedBoolType checks if t is a named bool type, e.g. type Flag bool, or a pointer to one. database/sql scans
// only bools into such fields, and SQLite returns integers.
func IsNamedBoolType(t reflect.Type) bool {
	t = BaseType(t)
	return t.Kind() == reflect.Bool && t != boolType
}

// IsTimeType checks if t is time.Time.
func IsTimeType(t reflect.Type) bool {
	return t == timeType
}

// IsFieldModification checks if a field is one of the fields that track the creation and the modification of an object:
// CreatedAt and ModifiedAt of type time.Time, or CreatedBy and ModifiedBy of type uint64.
func IsFieldModification(name string, t reflect.Type) bool {
	switch name {
	case "CreatedAt", "ModifiedAt":
		return IsTimeType(t)
	case "CreatedBy", "ModifiedBy":
		return t.Kind() == reflect.Uint64 || t.Kind() == reflect.String
	default:
		return false
	}
}
