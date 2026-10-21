package builder

import (
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mikogs/gocrud/pkg/structinfo"
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

// IsStructField checks if a field exists in a struct.
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

// SetStructFieldValueFromString sets a field from value that is string (converts it if necessary).
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

// PrettifyCreateTable prettifies SQL query to make it more human-readable.
func PrettifyCreateTable(sql string) string {
	sql = strings.Replace(sql, "(", "(\n  ", 1)
	sql = strings.ReplaceAll(sql, ",", ",\n  ")

	return sql
}

// MapInterfaces returns a slice of interfaces from a map.
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
	objValue := reflect.ValueOf(obj)
	objIndirect := reflect.Indirect(objValue)
	objType := objIndirect.Type()

	if objType.String() == "reflect.Value" {
		objType = reflect.ValueOf(obj.(reflect.Value).Interface()).Type().Elem()
	}

	for j := 0; j < objType.NumField(); j++ {
		field := objType.Field(j)
		kind := field.Type.Kind()

		if !field.IsExported() || !structinfo.IsFieldTypeSupported(field.Type) {
			continue
		}

		if field.Name == name {
			if structinfo.IsTimeType(field.Type) {
				v, ok := TimeFromString(value)
				if !ok {
					return false, nil
				}
				if set {
					objIndirect.Field(j).Set(reflect.ValueOf(v))
				}
				return true, v
			}

			switch kind {
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				if intRegex.MatchString(value) {
					v, err := strconv.ParseUint(value, 10, 64)
					if err == nil {
						if set {
							fieldValue := objIndirect.Field(j)
							if fieldValue.CanSet() {
								fieldValue.SetUint(v)
							}
						}
						return true, v
					}
				}
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if intRegex.MatchString(value) {
					v, err := strconv.ParseInt(value, 10, 64)
					if err == nil {
						if set {
							fieldValue := objIndirect.Field(j)
							if fieldValue.CanSet() {
								fieldValue.SetInt(v)
							}
						}
						return true, v
					}
				}
			case reflect.Float32, reflect.Float64:
				if floatRegex.MatchString(value) || intRegex.MatchString(value) {
					v, err := strconv.ParseFloat(value, 64)
					if err == nil {
						if set {
							fieldValue := objIndirect.Field(j)
							if fieldValue.CanSet() {
								fieldValue.SetFloat(v)
							}
						}
						return true, v
					}
				}
			case reflect.Bool:
				if strings.ToLower(value) == "true" || strings.ToLower(value) == "false" {
					v, err := strconv.ParseBool(value)
					if err == nil {
						if set {
							fieldValue := objIndirect.Field(j)
							if fieldValue.CanSet() {
								fieldValue.SetBool(v)
							}
						}
						return true, v
					}
				}
			case reflect.String:
				if set {
					fieldValue := objIndirect.Field(j)
					if fieldValue.CanSet() {
						fieldValue.SetString(value)
					}
				}
				return true, value
			}
			return false, nil
		}
	}

	return false, nil
}
