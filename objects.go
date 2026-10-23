package gocrud

import (
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/mikogs/gocrud/internal/structinfo"
)

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

// timeFieldsToUTC converts the values of obj time fields named in fieldNames to UTC. Databases return times
// in different time zones, such as the time zone of the database session.
func timeFieldsToUTC(obj interface{}, fieldNames []string) {
	objValue := reflect.ValueOf(obj).Elem()
	for _, fieldName := range fieldNames {
		field := objValue.FieldByName(fieldName)
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				continue
			}
			field = field.Elem()
		}
		field.Set(reflect.ValueOf(field.Interface().(time.Time).UTC()))
	}
}

func objIDInterface(obj interface{}) interface{} {
	return reflect.ValueOf(obj).Elem().FieldByName(IDField).Addr().Interface()
}

// ObjSetIDValue sets the ID of obj. For an ID of type uint64, id is a non-negative integer or a decimal string, and
// for a string ID, a UUID. nil clears the ID, so that the object is inserted as a new one. Otherwise, it returns an
// error that matches ErrInvalidID.
func ObjSetIDValue(obj interface{}, id interface{}) error {
	idField := reflect.ValueOf(obj).Elem().FieldByName(IDField)
	if id == nil {
		idField.Set(reflect.Zero(idField.Type()))
		return nil
	}

	value, err := idValue(idField.Kind() == reflect.String, id)
	if err != nil {
		return err
	}

	idField.Set(reflect.ValueOf(value))
	return nil
}

// idValue returns id as the value of an ID field: a UUID in its standard form for a string ID, and a uint64
// otherwise.
func idValue(isUUID bool, id interface{}) (interface{}, error) {
	if isUUID {
		var text string
		switch id := id.(type) {
		case string:
			text = id
		case uuid.UUID:
			text = id.String()
		default:
			return nil, fmt.Errorf("%w: %v (%T) is not a UUID", ErrInvalidID, id, id)
		}

		parsed, err := uuid.Parse(text)
		if err != nil {
			return nil, fmt.Errorf("%w: %q is not a UUID", ErrInvalidID, text)
		}
		return parsed.String(), nil
	}

	value := reflect.ValueOf(id)
	switch {
	case value.CanUint():
		return value.Uint(), nil
	case value.CanInt() && value.Int() >= 0:
		return uint64(value.Int()), nil // #nosec G115 -- the value is not negative
	case value.Kind() == reflect.String:
		number, err := strconv.ParseUint(value.String(), 10, 64)
		if err == nil {
			return number, nil
		}
	}

	return nil, fmt.Errorf("%w: %v (%T) is not a number", ErrInvalidID, id, id)
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

// ObjIDValue returns the ID of obj: a uint64, or a string with a UUID.
func ObjIDValue(obj interface{}) interface{} {
	return reflect.ValueOf(obj).Elem().FieldByName(IDField).Interface()
}

// ObjIDIsZero checks if the ID of obj is not set, i.e. 0 or an empty string, as in an object that is not saved.
func ObjIDIsZero(obj interface{}) bool {
	return reflect.ValueOf(obj).Elem().FieldByName(IDField).IsZero()
}

func objFieldValue(obj interface{}, fieldName string) interface{} {
	if obj == nil || fieldName == "" {
		return nil
	}

	// Get reflect.Value that we can inspect.
	// If we received a pointer, dereference it once to get the underlying struct.
	val := reflect.ValueOf(obj)

	// Handle pointers: we need the element the pointer points to.
	if val.Kind() == reflect.Pointer {
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

	// A nullable field returns the value it points to, or nil for NULL.
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return nil
		}
		field = field.Elem()
	}

	// Return the value only for the supported kinds.
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int() // returns int64, which satisfies interface{}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint()
	case reflect.Bool:
		return field.Bool()
	case reflect.Float32, reflect.Float64:
		return field.Float()
	case reflect.String:
		return field.String()
	case reflect.Struct:
		if structinfo.IsTimeType(field.Type()) {
			return field.Interface()
		}
		return nil
	default:
		// Unsupported type – caller gets nil.
		return nil
	}
}

// zeroObjFields sets all the exported fields of obj to their zero values.
func zeroObjFields(obj interface{}) {
	val := reflect.ValueOf(obj).Elem()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		if field.CanSet() {
			field.Set(reflect.Zero(field.Type()))
		}
	}
}

// setObjCreated sets CreatedAt and CreatedBy. by is the ID of the user, see setObjBy.
func setObjCreated(obj interface{}, at time.Time, by interface{}) error {
	reflect.ValueOf(obj).Elem().FieldByName("CreatedAt").Set(reflect.ValueOf(at))
	return setObjBy(obj, "CreatedBy", by)
}

// setObjModified sets ModifiedAt and ModifiedBy. by is the ID of the user, see setObjBy.
func setObjModified(obj interface{}, at time.Time, by interface{}) error {
	reflect.ValueOf(obj).Elem().FieldByName("ModifiedAt").Set(reflect.ValueOf(at))
	return setObjBy(obj, "ModifiedBy", by)
}

// setObjBy sets a field with the ID of a user: a non-negative integer for a uint64 field, and a string for a string
// field. nil sets the zero value. Otherwise, it returns an error that matches ErrInvalidID.
func setObjBy(obj interface{}, fieldName string, by interface{}) error {
	field := reflect.ValueOf(obj).Elem().FieldByName(fieldName)
	if by == nil {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}

	if field.Kind() == reflect.String {
		text, ok := by.(string)
		if !ok {
			return fmt.Errorf("%w: %s: %v (%T) is not a string", ErrInvalidID, fieldName, by, by)
		}
		field.SetString(text)
		return nil
	}

	value, err := idValue(false, by)
	if err != nil {
		return fmt.Errorf("%s: %w", fieldName, err)
	}
	field.SetUint(value.(uint64))
	return nil
}

func setObjStringField(obj interface{}, passField, hashField string) {
	reflect.ValueOf(obj).Elem().FieldByName(passField).SetString(hashField)
}
