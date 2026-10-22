package filters

import (
	"fmt"
	"reflect"

	"github.com/mikogs/gocrud/pkg/structinfo"
)

type Filters map[string]OpVal

func (f Filters) Add(name string, value OpVal) {
	f[name] = value
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
