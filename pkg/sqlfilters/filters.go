package sqlfilters

import (
	"fmt"
	"math"
	"reflect"

	"github.com/mikogs/gocrud/internal/structinfo"
)

type Filters map[string]OpVal

func (f Filters) Add(name string, value OpVal) {
	f[name] = value
}

func SetObjFields(obj any, values *Filters) error {
	if values == nil || len(*values) == 0 {
		return nil
	}

	// objValue must be a struct or point to a struct
	objValue := reflect.ValueOf(obj)
	if objValue.Kind() == reflect.Pointer {
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

		if !field.IsExported() {
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

		// e.g. sql.NullString, whose zero value is NULL
		if isNil(value.Val) && dest.Kind() != reflect.Pointer && structinfo.IsNullableType(dest.Type()) {
			dest.Set(reflect.Zero(dest.Type()))
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

// setValue sets dest to inVal when it is of the same type, when both are numbers and inVal fits in dest, or when both
// are strings or bools, e.g. a string for a field of type Status string.
func setValue(dest reflect.Value, inVal reflect.Value) bool {
	if !inVal.IsValid() {
		return false
	}

	if inVal.Type().AssignableTo(dest.Type()) {
		dest.Set(inVal)
		return true
	}

	// reflect also converts integers to strings (as runes), so conversion is limited to numbers
	if isKindNumeric(inVal.Kind()) && isKindNumeric(dest.Kind()) && numberFits(inVal, dest.Type()) {
		dest.Set(inVal.Convert(dest.Type()))
		return true
	}

	if inVal.Kind() == dest.Kind() && (dest.Kind() == reflect.String || dest.Kind() == reflect.Bool) {
		dest.Set(inVal.Convert(dest.Type()))
		return true
	}

	// e.g. a string on a field of a decimal type
	if structinfo.IsValuerType(dest.Type()) {
		if scanned, ok := structinfo.ScanValue(dest.Type(), inVal.Interface()); ok {
			dest.Set(scanned)
			return true
		}
	}

	return false
}

// numberFits checks if the number v is the same number after the conversion to the numeric type t: an integer in the
// range of t, a float that is a whole number in the range of an integer type t, or a number in the range of a float
// type t. reflect converts the numbers that do not fit without an error, e.g. 300 to 44 in an uint8.
func numberFits(v reflect.Value, t reflect.Type) bool {
	dest := reflect.New(t).Elem()

	switch {
	case v.CanInt():
		n := v.Int()
		switch {
		case dest.CanInt():
			return !dest.OverflowInt(n)
		case dest.CanUint():
			return n >= 0 && !dest.OverflowUint(uint64(n))
		}
		return true
	case v.CanUint():
		n := v.Uint()
		switch {
		case dest.CanInt():
			return n <= math.MaxInt64 && !dest.OverflowInt(int64(n))
		case dest.CanUint():
			return !dest.OverflowUint(n)
		}
		return true
	}

	f := v.Float()
	switch {
	case dest.CanInt():
		// -2^63 and 2^63 are exact as floats, unlike math.MaxInt64
		return f == math.Trunc(f) && f >= -(1<<63) && f < 1<<63 && !dest.OverflowInt(int64(f))
	case dest.CanUint():
		return f == math.Trunc(f) && f >= 0 && f < 1<<64 && !dest.OverflowUint(uint64(f))
	}
	return !dest.OverflowFloat(f)
}

func isNil(v any) bool {
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
