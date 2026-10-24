package sqlbuilder

import (
	"encoding"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mikogs/gocrud/internal/structinfo"
)

var intRegex = regexp.MustCompile(`^-?\d+$`)

// timeFormats are the formats that TimeFromString accepts. The ones without a time zone are used by
// HTML date and datetime-local inputs.
var timeFormats = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}
var floatRegex = regexp.MustCompile(`^-?\d*\.\d+$`)

func IsStructField(u interface{}, field string) bool {
	v := reflect.ValueOf(u)
	i := reflect.Indirect(v)
	s := i.Type()

	for j := 0; j < s.NumField(); j++ {
		f := s.Field(j)
		if !f.IsExported() || !structinfo.IsFieldTypeSupported(f.Type) {
			continue
		}

		if f.Name == field {
			return true
		}
	}

	return false
}

// StructFieldValueFromString takes a field value as string and converts it (if possible) to a value type of that field.
func StructFieldValueFromString(obj interface{}, name string, value string) (bool, interface{}) {
	return valueFromString(obj, name, value, false)
}

func SetStructFieldValueFromString(obj interface{}, name string, value string) (bool, interface{}) {
	return valueFromString(obj, name, value, true)
}

// TimeFromString converts a string in RFC 3339 format, such as "2024-05-01T10:30:00+02:00", or in one of the
// formats used by HTML date and datetime-local inputs ("2024-05-01", "2024-05-01T10:30") to time.Time.
// A time without a time zone is in UTC.
func TimeFromString(value string) (time.Time, bool) {
	for _, format := range timeFormats {
		t, err := time.Parse(format, value)
		if err == nil {
			return t, true
		}
	}

	return time.Time{}, false
}

func PrettifyCreateTable(sql string) string {
	sql = strings.Replace(sql, "(", "(\n  ", 1)
	sql = strings.ReplaceAll(sql, ",", ",\n  ")

	return sql
}

func MapInterfaces(mapObj map[string]interface{}) []interface{} {
	var interfaces []interface{}

	numObjs := len(mapObj)
	sorted := make([]string, 0, numObjs)
	for field := range mapObj {
		sorted = append(sorted, field)
	}
	sort.Strings(sorted)

	for _, val := range sorted {
		interfaces = append(interfaces, mapObj[val])
	}

	return interfaces
}

func valueFromString(obj interface{}, name string, value string, set bool) (bool, interface{}) {
	objIndirect := reflect.Indirect(reflect.ValueOf(obj))
	if objValue, ok := obj.(reflect.Value); ok {
		objIndirect = reflect.Indirect(objValue)
	}

	field, ok := objIndirect.Type().FieldByName(name)
	if !ok || !field.IsExported() || !structinfo.IsFieldTypeSupported(field.Type) {
		return false, nil
	}

	fieldType := field.Type
	// an empty value of a nullable field is NULL
	if value == "" && structinfo.IsNullableType(fieldType) {
		if fieldValue := objIndirect.FieldByIndex(field.Index); set && fieldValue.CanSet() {
			fieldValue.Set(reflect.Zero(fieldType))
		}
		return true, nil
	}
	fieldType = structinfo.BaseType(fieldType)

	converted, ok := convertFromString(fieldType, value)
	if !ok {
		return false, nil
	}

	if fieldValue := objIndirect.FieldByIndex(field.Index); set && fieldValue.CanSet() {
		typedValue := reflect.ValueOf(converted).Convert(fieldType)
		if fieldValue.Kind() == reflect.Pointer {
			pointer := reflect.New(fieldType)
			pointer.Elem().Set(typedValue)
			typedValue = pointer
		}
		fieldValue.Set(typedValue)
	}

	return true, converted
}

// valuerFromString converts value to a value of the Valuer type t: with its UnmarshalText method, e.g. of a UUID, or
// scanned from value converted to the type that t is written as, e.g. a time for sql.NullTime.
func valuerFromString(t reflect.Type, value string) (interface{}, bool) {
	pointer := reflect.New(t)
	if unmarshaler, ok := pointer.Interface().(encoding.TextUnmarshaler); ok {
		if err := unmarshaler.UnmarshalText([]byte(value)); err != nil {
			return nil, false
		}
		return pointer.Elem().Interface(), true
	}

	var converted interface{} = value
	if valueType := structinfo.ValuerValueType(t); valueType != nil {
		var ok bool
		if converted, ok = convertFromString(valueType, value); !ok {
			return nil, false
		}
	}

	scanned, ok := structinfo.ScanValue(t, converted)
	if !ok {
		return nil, false
	}
	return scanned.Interface(), true
}

// convertFromString converts value to a value of a type of the same kind as t: uint64 for unsigned integers,
// int64 for integers, float64 for floats, bool, string or time.Time. A number has to fit in t, e.g. 300 does not fit
// in an uint8, as it would be a different number after the conversion to t. A value of a Valuer type is a value of t.
func convertFromString(t reflect.Type, value string) (interface{}, bool) {
	if structinfo.IsTimeType(t) {
		return TimeFromString(value)
	}
	if structinfo.IsValuerType(t) {
		return valuerFromString(t, value)
	}

	switch t.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if intRegex.MatchString(value) {
			v, err := strconv.ParseUint(value, 10, t.Bits())
			if err == nil {
				return v, true
			}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if intRegex.MatchString(value) {
			v, err := strconv.ParseInt(value, 10, t.Bits())
			if err == nil {
				return v, true
			}
		}
	case reflect.Float32, reflect.Float64:
		if floatRegex.MatchString(value) || intRegex.MatchString(value) {
			v, err := strconv.ParseFloat(value, t.Bits())
			if err == nil {
				return v, true
			}
		}
	case reflect.Bool:
		if strings.ToLower(value) == "true" || strings.ToLower(value) == "false" {
			v, err := strconv.ParseBool(value)
			if err == nil {
				return v, true
			}
		}
	case reflect.String:
		return value, true
	}

	return nil, false
}
