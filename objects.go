package gocrud

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
	"github.com/mikogs/gocrud/internal/structinfo"
)

// objFieldPointers returns pointers to the fields of obj at indexes, in the same order, e.g. for Scan and the values of
// queries. The pointers are put in pointers when it has the length of indexes, so that it can be reused for every row.
func objFieldPointers(obj any, indexes []int, pointers []any) []any {
	if len(pointers) != len(indexes) {
		pointers = make([]any, len(indexes))
	}

	objValue := reflect.ValueOf(obj).Elem()
	for i, index := range indexes {
		pointers[i] = objValue.Field(index).Addr().Interface()
	}
	return pointers
}

// objScanPointers returns the destinations of Scan for the fields of obj at indexes, as objFieldPointers, except for
// the fields that database/sql cannot scan into on its own, by their kinds, which are Builder.FieldKinds.
func objScanPointers(obj any, indexes []int, kinds []sqlbuilder.FieldKind, pointers []any) []any {
	pointers = objFieldPointers(obj, indexes, pointers)
	if kinds == nil {
		return pointers
	}

	objValue := reflect.ValueOf(obj).Elem()
	for i, index := range indexes {
		switch kinds[index] {
		case sqlbuilder.FieldNamedBool:
			pointers[i] = namedBoolScanner{field: objValue.Field(index)}
		case sqlbuilder.FieldJSON:
			pointers[i] = jsonScanner{field: objValue.Field(index)}
		}
	}
	return pointers
}

// objArgs returns the values of the fields of obj at indexes for a query, as objFieldPointers, except for the fields
// that database/sql cannot write on its own, by their kinds, which are Builder.FieldKinds.
func objArgs(obj any, indexes []int, kinds []sqlbuilder.FieldKind) []any {
	args := objFieldPointers(obj, indexes, nil)
	if kinds == nil {
		return args
	}

	objValue := reflect.ValueOf(obj).Elem()
	for i, index := range indexes {
		switch kinds[index] {
		case sqlbuilder.FieldBytes:
			args[i] = bytesValuer{field: objValue.Field(index)}
		case sqlbuilder.FieldJSON:
			args[i] = jsonValuer{value: objValue.Field(index)}
		case sqlbuilder.FieldValuerPointer:
			// the Valuer itself, so that CRUD.args sees it
			if field := objValue.Field(index); field.IsNil() {
				args[i] = nil
			} else {
				args[i] = field.Interface()
			}
		}
	}
	return args
}

// bytesValuer writes a field of a []byte type that is not a pointer, which is stored in a NOT NULL column, as an empty
// value when it is nil, which the drivers write as NULL.
type bytesValuer struct {
	field reflect.Value
}

func (v bytesValuer) Value() (driver.Value, error) {
	if v.field.IsNil() {
		return []byte{}, nil
	}
	return v.field.Bytes(), nil
}

// jsonScanner scans JSON into a field stored as JSON. NULL sets the zero value.
type jsonScanner struct {
	field reflect.Value
}

func (s jsonScanner) Scan(src any) error {
	var data []byte
	switch v := src.(type) {
	case nil:
		s.field.Set(reflect.Zero(s.field.Type()))
		return nil
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into %s as JSON", src, s.field.Type())
	}

	// a new value, as encoding/json merges JSON into the maps and the structs that a field already has
	value := reflect.New(s.field.Type())
	if err := json.Unmarshal(data, value.Interface()); err != nil {
		return fmt.Errorf("cannot scan JSON into %s: %w", s.field.Type(), err)
	}
	s.field.Set(value.Elem())
	return nil
}

// jsonValuer writes the value of a field stored as JSON, as text, which both JSONB and TEXT columns accept, or NULL
// for a nil pointer.
type jsonValuer struct {
	value reflect.Value
}

func (v jsonValuer) Value() (driver.Value, error) {
	value := v.value
	if !value.IsValid() || value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, nil
	}
	// the MarshalJSON methods with pointer receivers are called on fields that are not pointers too
	if value.CanAddr() {
		value = value.Addr()
	}

	data, err := json.Marshal(value.Interface())
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// namedBoolScanner scans a value into a field of a named bool type, or a pointer to one, with the conversions of
// driver.Bool, e.g. from the integers of SQLite.
type namedBoolScanner struct {
	field reflect.Value
}

func (s namedBoolScanner) Scan(src any) error {
	field := s.field
	if src == nil {
		if field.Kind() != reflect.Pointer {
			return fmt.Errorf("cannot scan NULL into %s", field.Type())
		}
		field.Set(reflect.Zero(field.Type()))
		return nil
	}

	value, err := driver.Bool.ConvertValue(src)
	if err != nil {
		return err
	}
	if field.Kind() == reflect.Pointer {
		field.Set(reflect.New(field.Type().Elem()))
		field = field.Elem()
	}
	field.SetBool(value.(bool))
	return nil
}

// timeFieldsToUTC converts the values of the time fields of obj at indexes to UTC. Databases return times in
// different time zones, such as the time zone of the database session.
func timeFieldsToUTC(obj any, indexes []int) {
	objValue := reflect.ValueOf(obj).Elem()
	for _, index := range indexes {
		field := objValue.Field(index)
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				continue
			}
			field = field.Elem()
		}
		field.Set(reflect.ValueOf(field.Interface().(time.Time).UTC()))
	}
}

func objIDInterface(obj any) any {
	return reflect.ValueOf(obj).Elem().FieldByName(IDField).Addr().Interface()
}

// ObjSetIDValue sets the ID of obj. For an ID of type uint64, id is a non-negative integer or a decimal string, and
// for a string ID, a UUID. nil clears the ID, so that the object is inserted as a new one. Otherwise, it returns an
// error that matches ErrInvalidID.
func ObjSetIDValue(obj any, id any) error {
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
func idValue(isUUID bool, id any) (any, error) {
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

// zeroPasswordFields clears the password fields of obj at indexes, so that bcrypt hashes are never returned from
// read operations.
func zeroPasswordFields(obj any, indexes []int) {
	if len(indexes) == 0 {
		return
	}
	v := reflect.ValueOf(obj).Elem()
	for _, index := range indexes {
		v.Field(index).SetString("")
	}
}

// ObjIDValue returns the ID of obj: a uint64, or a string with a UUID.
func ObjIDValue(obj any) any {
	return reflect.ValueOf(obj).Elem().FieldByName(IDField).Interface()
}

// ObjIDIsZero checks if the ID of obj is not set, i.e. 0 or an empty string, as in an object that is not saved.
func ObjIDIsZero(obj any) bool {
	return reflect.ValueOf(obj).Elem().FieldByName(IDField).IsZero()
}

func objFieldValue(obj any, fieldName string) any {
	if obj == nil || fieldName == "" {
		return nil
	}

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil
	}

	field := val.FieldByName(fieldName)
	if !field.IsValid() || !field.CanInterface() {
		return nil
	}

	// A nullable field returns the value it points to, or nil for NULL.
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return nil
		}
		field = field.Elem()
	}

	if structinfo.IsValuerType(field.Type()) {
		// e.g. sql.NullString
		valuer, ok := field.Interface().(driver.Valuer)
		if !ok {
			valuer = field.Addr().Interface().(driver.Valuer)
		}
		if value, err := valuer.Value(); err != nil || value == nil {
			return nil
		}
		return field.Interface()
	}

	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int()
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
		return nil
	}
}

// zeroObjFields sets all the exported fields of obj to their zero values.
func zeroObjFields(obj any) {
	val := reflect.ValueOf(obj).Elem()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		if field.CanSet() {
			field.Set(reflect.Zero(field.Type()))
		}
	}
}

// setObjCreated sets CreatedAt and CreatedBy. by is the ID of the user, see setObjBy.
func setObjCreated(obj any, at time.Time, by any) error {
	reflect.ValueOf(obj).Elem().FieldByName("CreatedAt").Set(reflect.ValueOf(at))
	return setObjBy(obj, "CreatedBy", by)
}

// setObjModified sets ModifiedAt and ModifiedBy. by is the ID of the user, see setObjBy.
func setObjModified(obj any, at time.Time, by any) error {
	reflect.ValueOf(obj).Elem().FieldByName("ModifiedAt").Set(reflect.ValueOf(at))
	return setObjBy(obj, "ModifiedBy", by)
}

// setObjBy sets a field with the ID of a user: a non-negative integer for a uint64 field, and a string for a string
// field. nil sets the zero value. Otherwise, it returns an error that matches ErrInvalidID.
func setObjBy(obj any, fieldName string, by any) error {
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

func setObjStringField(obj any, passField, hashField string) {
	reflect.ValueOf(obj).Elem().FieldByName(passField).SetString(hashField)
}
