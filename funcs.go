package gocrud

import (
	"reflect"

	structInfo "github.com/mikogs/gocrud/pkg/structinfo"
)

// ObjFieldInterfaces returns a list of interfaces to object's fields
// Argument includeID tells it to include or omit the ID field.
// Fields ignored with a "-" tag are included, as the tags are not read.
func ObjFieldInterfaces(obj interface{}, includeID bool) []interface{} {
	objValue := reflect.ValueOf(obj).Elem()

	var fieldInterfaces []interface{}

	for i := 0; i < objValue.NumField(); i++ {
		valueField := objValue.Field(i)
		if objValue.Type().Field(i).Name == IDField && !includeID {
			continue
		}

		// builder is used to generate SQL queries, so here the same kinds must be supported
		if !objValue.Type().Field(i).IsExported() || !structInfo.IsFieldKindSupported(valueField.Kind()) {
			continue
		}

		fieldInterfaces = append(fieldInterfaces, valueField.Addr().Interface())
	}

	return fieldInterfaces
}

// objColumnInterfaces returns a list of interfaces to object's fields named in fieldNames, in the same order.
// Argument includeID tells it to include or omit the ID field.
func objColumnInterfaces(obj interface{}, fieldNames []string, includeID bool) []interface{} {
	objValue := reflect.ValueOf(obj).Elem()

	fieldInterfaces := make([]interface{}, 0, len(fieldNames))
	for _, fieldName := range fieldNames {
		if fieldName == IDField && !includeID {
			continue
		}

		fieldInterfaces = append(fieldInterfaces, objValue.FieldByName(fieldName).Addr().Interface())
	}

	return fieldInterfaces
}

func ObjIDInterface(obj interface{}) interface{} {
	return reflect.ValueOf(obj).Elem().FieldByName(IDField).Addr().Interface()
}

func ObjSetIDValue(obj interface{}, id uint64) {
	reflect.ValueOf(obj).Elem().FieldByName(IDField).SetUint(id)
}

// zeroPasswordFields clears the value of every password field on obj so that
// bcrypt hashes are never returned from read operations.
func zeroPasswordFields(obj interface{}, passwordFields []string) {
	if len(passwordFields) == 0 {
		return
	}
	v := reflect.ValueOf(obj).Elem()
	for _, name := range passwordFields {
		f := v.FieldByName(name)
		if f.IsValid() && f.CanSet() {
			f.SetString("")
		}
	}
}

func ObjIDValue(obj interface{}) uint64 {
	return reflect.ValueOf(obj).Elem().FieldByName(IDField).Uint()
}

func ObjFieldValue(obj interface{}, fieldName string) interface{} {
	if obj == nil || fieldName == "" {
		return nil
	}

	// Get reflect.Value that we can inspect.
	// If we received a pointer, dereference it once to get the underlying struct.
	val := reflect.ValueOf(obj)

	// Handle pointers: we need the element the pointer points to.
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil // nil pointer – nothing to read
		}
		val = val.Elem()
	}

	// We must be dealing with a struct at this point.
	if val.Kind() != reflect.Struct {
		return nil
	}

	// Look up the field by name.
	field := val.FieldByName(fieldName)
	if !field.IsValid() {
		// No such field.
		return nil
	}

	// Unexported fields are not accessible via Interface().
	// The CanInterface check protects us from a panic.
	if !field.CanInterface() {
		return nil
	}

	// Return the value only for the supported kinds.
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int() // returns int64, which satisfies interface{}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint()
	case reflect.Bool:
		return field.Bool()
	case reflect.String:
		return field.String()
	default:
		// Unsupported type – caller gets nil.
		return nil
	}
}

func ZeroObjFields(obj interface{}) {
	val := reflect.ValueOf(obj).Elem()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		if !field.CanSet() {
			continue
		}

		kind := field.Kind()

		if kind == reflect.Ptr {
			field.Set(reflect.Zero(field.Type()))
		}
		if kind == reflect.Int || kind == reflect.Int8 || kind == reflect.Int16 || kind == reflect.Int32 || kind == reflect.Int64 {
			field.SetInt(0)
		}
		if kind == reflect.Uint || kind == reflect.Uint8 || kind == reflect.Uint16 || kind == reflect.Uint32 || kind == reflect.Uint64 {
			field.SetUint(0)
		}
		if kind == reflect.Float32 || kind == reflect.Float64 {
			field.SetFloat(0.0)
		}
		if kind == reflect.String {
			field.SetString("")
		}
		if kind == reflect.Bool {
			field.SetBool(false)
		}
	}
}

func SetObjCreated(obj interface{}, at int64, by uint64) {
	reflect.ValueOf(obj).Elem().FieldByName("CreatedAt").SetInt(at)
	reflect.ValueOf(obj).Elem().FieldByName("CreatedBy").SetUint(by)
}

func SetObjModified(obj interface{}, at int64, by uint64) {
	reflect.ValueOf(obj).Elem().FieldByName("ModifiedAt").SetInt(at)
	reflect.ValueOf(obj).Elem().FieldByName("ModifiedBy").SetUint(by)
}

func SetObjStringField(obj interface{}, passField, hashField string) {
	reflect.ValueOf(obj).Elem().FieldByName(passField).SetString(hashField)
}
