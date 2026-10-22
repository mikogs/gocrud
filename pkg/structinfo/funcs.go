package structinfo

import (
	"reflect"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})

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
// or a pointer to one of them, which is stored in a nullable column.
func IsFieldTypeSupported(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return IsTimeType(t) || IsFieldKindSupported(t.Kind())
}

// IsNullableType checks if a field of a specific type is stored in a nullable column, i.e. it is a pointer.
func IsNullableType(t reflect.Type) bool {
	return t.Kind() == reflect.Pointer && IsFieldTypeSupported(t.Elem())
}

// BaseType returns the type that a nullable field points to, or t itself for a field that is not nullable.
func BaseType(t reflect.Type) reflect.Type {
	if IsNullableType(t) {
		return t.Elem()
	}

	return t
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
