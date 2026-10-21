package gocrud

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sync"

	"github.com/go-playground/validator/v10"
	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

const (
	// ViolationType is set when a value cannot be assigned to a field because of its type.
	ViolationType = "type"
	// ViolationRegExp is set when a value does not match the pattern from the field's <tagName>_regexp tag.
	ViolationRegExp = "regexp"
)

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
		if !field.IsExported() || field.Type.Kind() != reflect.String || violations[field.Name] != "" {
			continue
		}

		if len(restrictFields) > 0 && !restrictFields[field.Name] {
			continue
		}

		pattern := field.Tag.Get(regexpTagName)
		if pattern == "" {
			continue
		}

		compiled, err := getCompiledRegexp(pattern)
		// invalid pattern is considered a failed regexp
		if err != nil || !compiled.MatchString(val.Field(i).String()) {
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
		if !valueVal.IsValid() || !valueVal.Type().AssignableTo(field.Type()) {
			typeViolations[name] = ViolationType
			continue
		}

		field.Set(valueVal)
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

	err := sqlfilters.SetObjFields(obj, filters)
	if err != nil {
		return getBuilderFuncCRUDError("set obj fields", err)
	}

	fieldsToValidate := map[string]bool{}
	for filter := range *filters {
		fieldsToValidate[filter] = true
	}

	ok, violations, err := Validate(obj, fieldsToValidate, tagName)
	if err != nil {
		return getValidateObjCRUDError(err)
	}

	if !ok {
		return getObjInvalidCRUDError(violations)
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
