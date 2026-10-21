package gocrud

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	sqlbuilder "github.com/mikogs/gocrud/pkg/sql/builder"
	"github.com/mikogs/gocrud/pkg/structinfo"
)

const (
	// ViolationType is set when a value cannot be assigned to a field because of its type.
	ViolationType = "type"
	// ViolationRegExp is set when a value does not match the pattern from the field's <tagName>_regexp tag.
	ViolationRegExp = "regexp"
)

const validateTagName = "validate"

var (
	structValidator = validator.New()
	regexpCache     sync.Map
)

// Validate validates obj field values against their go-playground `validate` tags and their
// <tagName>_regexp tags. If restrictFields is not empty, only the listed fields are validated.
// Violations map field names to the name of the failed validation tag, eg. "required" or "min".
func Validate(obj interface{}, restrictFields map[string]bool, tagName string) (bool, map[string]string, error) {
	val := reflect.ValueOf(obj)
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return false, nil, errors.New("nil pointer supplied")
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return false, nil, fmt.Errorf("obj is not a struct or pointer to struct, got %s", val.Kind())
	}

	var err error
	if len(restrictFields) > 0 {
		fields := make([]string, 0, len(restrictFields))
		for name, ok := range restrictFields {
			if ok {
				fields = append(fields, name)
			}
		}
		err = structValidator.StructPartial(obj, fields...)
	} else {
		err = structValidator.Struct(obj)
	}

	violations := map[string]string{}

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		for _, fieldErr := range validationErrs {
			violations[fieldErr.StructField()] = fieldErr.Tag()
		}
	} else if err != nil {
		return false, nil, err
	}

	// regexp patterns are kept in a separate tag as they often contain characters
	// that have a special meaning in the `validate` tag, such as ',' and '|'
	regexpTagName := tagName + "_regexp"
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() || structinfo.BaseType(field.Type).Kind() != reflect.String || violations[field.Name] != "" {
			continue
		}

		if len(restrictFields) > 0 && !restrictFields[field.Name] {
			continue
		}

		pattern := field.Tag.Get(regexpTagName)
		if pattern == "" {
			continue
		}

		validateTag := field.Tag.Get(validateTagName)
		if validateTag == "-" {
			continue
		}

		fieldValue := val.Field(i)
		if fieldValue.Kind() == reflect.Pointer {
			// NULL does not have a value to match
			if fieldValue.IsNil() {
				continue
			}
			fieldValue = fieldValue.Elem()
		}

		value := fieldValue.String()
		if value == "" && strings.Split(validateTag, ",")[0] == "omitempty" {
			continue
		}

		compiled, err := getCompiledRegexp(pattern)
		// invalid pattern is considered a failed regexp
		if err != nil || !compiled.MatchString(value) {
			violations[field.Name] = ViolationRegExp
		}
	}

	return len(violations) == 0, violations, nil
}

// ValidateValues validates values as if they were set on obj fields, without modifying obj.
// Only the fields present in values are validated.
func ValidateValues(obj interface{}, values map[string]interface{}, tagName string) (bool, map[string]string, error) {
	val := reflect.ValueOf(obj)
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return false, nil, errors.New("nil pointer supplied")
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return false, nil, fmt.Errorf("obj is not a struct or pointer to struct, got %s", val.Kind())
	}

	objCopy := reflect.New(val.Type())
	objCopy.Elem().Set(val)

	typeViolations := map[string]string{}
	restrictFields := make(map[string]bool, len(values))
	for name, value := range values {
		field := objCopy.Elem().FieldByName(name)
		if !field.IsValid() || !field.CanSet() {
			continue
		}

		valueVal := reflect.ValueOf(value)
		switch {
		case valueVal.IsValid() && valueVal.Type().AssignableTo(field.Type()):
			field.Set(valueVal)
		case field.Kind() == reflect.Pointer && !valueVal.IsValid():
			// NULL
			field.Set(reflect.Zero(field.Type()))
		case field.Kind() == reflect.Pointer && valueVal.Type().AssignableTo(field.Type().Elem()):
			pointer := reflect.New(field.Type().Elem())
			pointer.Elem().Set(valueVal)
			field.Set(pointer)
		default:
			typeViolations[name] = ViolationType
			continue
		}

		restrictFields[name] = true
	}

	if len(restrictFields) == 0 {
		return len(typeViolations) == 0, typeViolations, nil
	}

	_, violations, err := Validate(objCopy.Interface(), restrictFields, tagName)
	if err != nil {
		return false, nil, err
	}

	for name, violation := range typeViolations {
		violations[name] = violation
	}

	return len(violations) == 0, violations, nil
}

func ValidateFilters(obj interface{}, filters *sqlfilters.Filters, tagName string) error {
	if filters == nil || len(*filters) == 0 {
		return nil
	}

	// IS NULL and IS NOT NULL conditions do not have a value to validate, but they are valid only for nullable fields
	valueFilters := sqlfilters.Filters{}
	inFilters := sqlfilters.Filters{}
	nullViolations := map[string]string{}
	objType := reflect.Indirect(reflect.ValueOf(obj)).Type()
	for name, filter := range *filters {
		field, isField := objType.FieldByName(name)

		if filter.Op == sqlfilters.OpIn && name != sqlfilters.Raw {
			inFilters[name] = filter
			continue
		}

		if !filter.IsNull() {
			// nil is valid only with OpEqual and OpNotEqual, as IS NULL and IS NOT NULL
			if isField && isNilValue(filter.Val) {
				nullViolations[name] = ViolationType
				continue
			}
			valueFilters[name] = filter
			continue
		}

		if isField && !structinfo.IsNullableType(field.Type) {
			nullViolations[name] = ViolationType
		}
	}

	if len(nullViolations) > 0 {
		return getObjInvalidCRUDError(nullViolations)
	}

	if len(valueFilters) > 0 {
		err := sqlfilters.SetObjFields(obj, &valueFilters)
		if err != nil {
			return getBuilderFuncCRUDError("set obj fields", err)
		}

		fieldsToValidate := map[string]bool{}
		for filter := range valueFilters {
			fieldsToValidate[filter] = true
		}

		ok, violations, err := Validate(obj, fieldsToValidate, tagName)
		if err != nil {
			return getValidateObjCRUDError(err)
		}

		if !ok {
			return getObjInvalidCRUDError(violations)
		}
	}

	for name, filter := range inFilters {
		err := validateInFilter(obj, name, filter, tagName)
		if err != nil {
			return err
		}
	}

	return nil
}

// validateInFilter validates each value of an OpIn filter as if it was the value of the field.
func validateInFilter(obj interface{}, name string, filter sqlfilters.OpVal, tagName string) error {
	values, ok := filter.InValues()
	if !ok {
		return getObjInvalidCRUDError(map[string]string{name: ViolationType})
	}

	for _, value := range values {
		// NULL is never IN a list
		if isNilValue(value) {
			return getObjInvalidCRUDError(map[string]string{name: ViolationType})
		}

		err := sqlfilters.SetObjFields(obj, &sqlfilters.Filters{name: {Op: sqlfilters.OpEqual, Val: value}})
		if err != nil {
			return getObjInvalidCRUDError(map[string]string{name: ViolationType})
		}

		ok, violations, err := Validate(obj, map[string]bool{name: true}, tagName)
		if err != nil {
			return getValidateObjCRUDError(err)
		}

		if !ok {
			return getObjInvalidCRUDError(violations)
		}
	}

	return nil
}

// getCompiledRegexp returns a compiled regexp for the pattern, caching it for subsequent calls.
func getCompiledRegexp(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexpCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	regexpCache.Store(pattern, compiled)
	return compiled, nil
}

// convertFiltersFromString returns filters with string values converted to the types of obj fields. The raw filter,
// and IS NULL and IS NOT NULL conditions, which have a nil value, are not converted.
func convertFiltersFromString(obj interface{}, filters *sqlfilters.Filters) (*sqlfilters.Filters, error) {
	if filters == nil {
		return nil, nil
	}

	newFilters := &sqlfilters.Filters{}
	for filterName, filterOpVal := range *filters {
		if filterName == sqlfilters.Raw || filterOpVal.IsNull() {
			newFilters.Add(filterName, filterOpVal)
			continue
		}

		valueAsString, ok := filterOpVal.Val.(string)
		if !ok {
			return nil, getObjInvalidCRUDError(map[string]string{filterName: ViolationType})
		}

		// the values of IN are separated with commas, and an empty string is an empty list
		if filterOpVal.Op == sqlfilters.OpIn {
			values := []interface{}{}
			if valueAsString != "" {
				for _, part := range strings.Split(valueAsString, ",") {
					ok, value := sqlbuilder.StructFieldValueFromString(obj, filterName, part)
					if !ok {
						return nil, getObjInvalidCRUDError(map[string]string{filterName: ViolationType})
					}
					values = append(values, value)
				}
			}

			newFilters.Add(filterName, sqlfilters.OpVal{Op: filterOpVal.Op, Val: values})
			continue
		}

		ok, valueAsFieldType := sqlbuilder.StructFieldValueFromString(obj, filterName, valueAsString)
		if !ok {
			return nil, getObjInvalidCRUDError(map[string]string{filterName: ViolationType})
		}

		newFilters.Add(filterName, sqlfilters.OpVal{
			Op:  filterOpVal.Op,
			Val: valueAsFieldType,
		})
	}

	return newFilters, nil
}

func isNilValue(v interface{}) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}
