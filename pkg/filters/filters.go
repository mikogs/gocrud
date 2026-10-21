package filters

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/mikogs/gocrud/pkg/structinfo"
)

type Filters map[string]OpVal

func (f Filters) Add(name string, value OpVal) {
	f[name] = value
}

const (
	Raw = "_"
)

// FiltersInterfaces returns a list of interfaces from a filters map (used in querying)
func FiltersInterfaces(filters *Filters) []interface{} {
	var interfaces []interface{}

	if filters == nil || len(*filters) == 0 {
		return interfaces
	}

	numFilters := len(*filters)
	sorted := make([]string, 0, numFilters)
	for k := range *filters {
		if k == Raw {
			continue
		}
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	for _, filter := range sorted {
		// IS NULL and IS NOT NULL conditions do not have a value
		if (*filters)[filter].IsNull() {
			continue
		}
		interfaces = append(interfaces, (*filters)[filter].Val)
	}

	// Get pointers to values from raw query
	rawFilters, ok := (*filters)[Raw]
	if !ok {
		return interfaces
	}

	// The first raw item in the array is a query. Invalid raw filters are rejected when the query is built.
	rawValues, ok := rawFilters.Val.([]interface{})
	if !ok || len(rawValues) < 2 {
		return interfaces
	}

	for i := 1; i < len(rawValues); i++ {
		placeholderValue := reflect.ValueOf(rawValues[i])

		if placeholderValue.Kind() != reflect.Slice && placeholderValue.Kind() != reflect.Array {
			interfaces = append(interfaces, rawValues[i])

			continue
		}

		valInterfaces, ok := rawValues[i].([]interface{})
		if ok {
			for j := 0; j < len(valInterfaces); j++ {
				interfaces = append(interfaces, valInterfaces[j])
			}
			continue
		}
		valInt8s, ok := rawValues[i].([]int8)
		if ok {
			for j := 0; j < len(valInt8s); j++ {
				interfaces = append(interfaces, valInt8s[j])
			}
			continue
		}
		valInt16s, ok := rawValues[i].([]int16)
		if ok {
			for j := 0; j < len(valInt16s); j++ {
				interfaces = append(interfaces, valInt16s[j])
			}
			continue
		}
		valInt32s, ok := rawValues[i].([]int32)
		if ok {
			for j := 0; j < len(valInt32s); j++ {
				interfaces = append(interfaces, valInt32s[j])
			}
			continue
		}
		valInt64s, ok := rawValues[i].([]int64)
		if ok {
			for j := 0; j < len(valInt64s); j++ {
				interfaces = append(interfaces, valInt64s[j])
			}
			continue
		}
		valInts, ok := rawValues[i].([]int)
		if ok {
			for j := 0; j < len(valInts); j++ {
				interfaces = append(interfaces, valInts[j])
			}
			continue
		}
		valUint8s, ok := rawValues[i].([]uint8)
		if ok {
			for j := 0; j < len(valUint8s); j++ {
				interfaces = append(interfaces, valUint8s[j])
			}
			continue
		}
		valUint16s, ok := rawValues[i].([]uint16)
		if ok {
			for j := 0; j < len(valUint16s); j++ {
				interfaces = append(interfaces, valUint16s[j])
			}
			continue
		}
		valUint32s, ok := rawValues[i].([]uint32)
		if ok {
			for j := 0; j < len(valUint32s); j++ {
				interfaces = append(interfaces, valUint32s[j])
			}
			continue
		}
		valUint64s, ok := rawValues[i].([]uint64)
		if ok {
			for j := 0; j < len(valUint64s); j++ {
				interfaces = append(interfaces, valUint64s[j])
			}
			continue
		}
		valUints, ok := rawValues[i].([]uint)
		if ok {
			for j := 0; j < len(valUints); j++ {
				interfaces = append(interfaces, valUints[j])
			}
			continue
		}
		valFloat32s, ok := rawValues[i].([]float32)
		if ok {
			for j := 0; j < len(valFloat32s); j++ {
				interfaces = append(interfaces, valFloat32s[j])
			}
			continue
		}
		valFloat64s, ok := rawValues[i].([]float64)
		if ok {
			for j := 0; j < len(valFloat64s); j++ {
				interfaces = append(interfaces, valFloat64s[j])
			}
			continue
		}
		valBools, ok := rawValues[i].([]bool)
		if ok {
			for j := 0; j < len(valBools); j++ {
				interfaces = append(interfaces, valBools[j])
			}
			continue
		}
		valStrings, ok := rawValues[i].([]string)
		if ok {
			for j := 0; j < len(valStrings); j++ {
				interfaces = append(interfaces, valStrings[j])
			}
		}
	}

	return interfaces
}

func SetObjFields(obj interface{}, values *Filters) error {
	if values == nil || len(*values) == 0 {
		return nil
	}

	// objValue must be a struct or point to a struct
	objValue := reflect.ValueOf(obj)
	if objValue.Kind() == reflect.Ptr {
		if objValue.IsNil() {
			return fmt.Errorf("obj cannot be a nil pointer")
		}

		objValue = objValue.Elem()
	}

	if objValue.Kind() != reflect.Struct {
		return fmt.Errorf("obj must be or point to a struct, got %s", objValue.Kind())
	}

	var firstErr error
	typ := objValue.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		if field.PkgPath != "" { // PkgPath is non‑empty for unexported fields
			continue
		}

		if !structinfo.IsFieldTypeSupported(field.Type) {
			continue
		}

		value, ok := (*values)[field.Name]
		if !ok {
			continue
		}

		dest := objValue.Field(i)
		inVal := reflect.ValueOf(value.Val)

		if setValue(dest, inVal) {
			continue
		}

		// a nullable field is set to nil, or to a pointer to a value of the type it points to
		if dest.Kind() == reflect.Pointer {
			if isNil(value.Val) {
				dest.Set(reflect.Zero(dest.Type()))
				continue
			}

			if inVal.Kind() == reflect.Pointer {
				inVal = inVal.Elem()
			}

			elem := reflect.New(dest.Type().Elem())
			if setValue(elem.Elem(), inVal) {
				dest.Set(elem)
				continue
			}
		}

		err := fmt.Errorf("cannot set field %s (%s) with value of type %T",
			field.Name, dest.Type(), value.Val)
		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// setValue sets dest to inVal when it is of the same type, or when both are numbers.
func setValue(dest reflect.Value, inVal reflect.Value) bool {
	if !inVal.IsValid() {
		return false
	}

	if inVal.Type().AssignableTo(dest.Type()) {
		dest.Set(inVal)
		return true
	}

	// reflect also converts integers to strings (as runes), so conversion is limited to numbers
	if isKindNumeric(inVal.Kind()) && isKindNumeric(dest.Kind()) {
		dest.Set(inVal.Convert(dest.Type()))
		return true
	}

	return false
}

func isNil(v interface{}) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

func isKindNumeric(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
