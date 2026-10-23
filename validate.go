package gocrud

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/mikogs/gocrud/internal/structinfo"
)

const (
	// ViolationType is set when a value cannot be assigned to a field because of its type.
	ViolationType = "type"
	// ViolationRegExp is set when a value does not match the pattern from the field's <tagName>_regexp tag.
	ViolationRegExp = "regexp"
	// ViolationPassword is set for a condition of a filter on a password field, which cannot be used in filters.
	ViolationPassword = "password"
)

const validateTagName = "validate"

var (
	structValidator    = validator.New()
	regexpCache        sync.Map
	relationFieldCache sync.Map
)

type relationFieldCacheKey struct {
	typ     reflect.Type
	tagName string
}

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

	// related objects are validated when they are saved, and not as a part of the object that holds them
	relationFields := getRelationFieldNames(val.Type(), tagName)

	var err error
	switch {
	case len(restrictFields) > 0:
		fields := make([]string, 0, len(restrictFields))
		for name, ok := range restrictFields {
			if ok && !relationFields[name] {
				fields = append(fields, name)
			}
		}
		err = structValidator.StructPartial(obj, fields...)
	case len(relationFields) > 0:
		fields := make([]string, 0, len(relationFields))
		for name := range relationFields {
			fields = append(fields, name)
		}
		err = structValidator.StructExcept(obj, fields...)
	default:
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

// getRelationFieldNames returns the names of the relation fields of the struct type t, caching them for subsequent calls.
func getRelationFieldNames(t reflect.Type, tagName string) map[string]bool {
	key := relationFieldCacheKey{typ: t, tagName: tagName}
	if v, ok := relationFieldCache.Load(key); ok {
		return v.(map[string]bool)
	}

	names := map[string]bool{}
	for _, name := range structinfo.RelationFieldNames(t, tagName) {
		names[name] = true
	}

	relationFieldCache.Store(key, names)
	return names
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

func isNilValue(v interface{}) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}
