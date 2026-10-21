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

// IsFieldTypeSupported checks if a field of a specific type can be stored in the database.
func IsFieldTypeSupported(t reflect.Type) bool {
	return IsTimeType(t) || IsFieldKindSupported(t.Kind())
}

// IsTimeType checks if t is time.Time.
func IsTimeType(t reflect.Type) bool {
	return t == timeType
}

func IsFieldModification(name string, typeKind reflect.Kind) bool {
	return ((name == "CreatedBy" || name == "ModifiedBy") && typeKind == reflect.Uint64) || ((name == "CreatedAt" || name == "ModifiedAt") && typeKind == reflect.Int64)
}
