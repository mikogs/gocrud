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
	"github.com/mikogs/gocrud/pkg/sqlfilters"
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
	structValidator     = validator.New()
	regexpCache         sync.Map
	validationRuleCache sync.Map
)

type validationRuleCacheKey struct {
	typ     reflect.Type
	tagName string
}

// validationRules are the rules of a struct type, so that its tags are not read for every validation
type validationRules struct {
	// related objects are validated when they are saved, and not as a part of the object that holds them
	relationFields map[string]bool
	// the fields that go-playground validates: with a `validate` tag, or with a struct that has them
	validatedFields map[string]bool
	regexpFields    []regexpRule
}

type regexpRule struct {
	index     int
	name      string
	pattern   string
	omitempty bool
}

// Validate validates obj field values against their go-playground `validate` tags and their
// <tagName>_regexp tags. If restrictFields is not empty, only the listed fields are validated.
// Violations map field names to the name of the failed validation tag, eg. "required" or "min".
func Validate(obj any, restrictFields map[string]bool, tagName string) (bool, map[string]string, error) {
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

	rules := getValidationRules(val.Type(), tagName)

	var err error
	switch {
	case len(restrictFields) > 0:
		var fields []string
		for name, ok := range restrictFields {
			if ok && rules.validatedFields[name] {
				fields = append(fields, name)
			}
		}
		if len(fields) > 0 {
			err = structValidator.StructPartial(obj, fields...)
		}
	case len(rules.validatedFields) == 0:
	case len(rules.relationFields) > 0:
		fields := make([]string, 0, len(rules.relationFields))
		for name := range rules.relationFields {
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

	for _, rule := range rules.regexpFields {
		if violations[rule.name] != "" || (len(restrictFields) > 0 && !restrictFields[rule.name]) {
			continue
		}

		fieldValue := val.Field(rule.index)
		if fieldValue.Kind() == reflect.Pointer {
			// NULL does not have a value to match
			if fieldValue.IsNil() {
				continue
			}
			fieldValue = fieldValue.Elem()
		}

		value := fieldValue.String()
		if value == "" && rule.omitempty {
			continue
		}

		compiled, err := getCompiledRegexp(rule.pattern)
		// invalid pattern is considered a failed regexp
		if err != nil || !compiled.MatchString(value) {
			violations[rule.name] = ViolationRegExp
		}
	}

	return len(violations) == 0, violations, nil
}

// ValidateValues validates values as if they were set on obj fields, without modifying obj.
// Only the fields present in values are validated.
func ValidateValues(obj any, values map[string]any, tagName string) (bool, map[string]string, error) {
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

		// the values are set as the values of filters, e.g. a string on a field of type Status string
		if err := sqlfilters.SetObjFields(objCopy.Interface(), &sqlfilters.Filters{name: {Op: sqlfilters.OpEqual, Val: value}}); err != nil {
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

// getValidationRules returns the validation rules of the struct type t, caching them for subsequent calls.
func getValidationRules(t reflect.Type, tagName string) *validationRules {
	key := validationRuleCacheKey{typ: t, tagName: tagName}
	if v, ok := validationRuleCache.Load(key); ok {
		return v.(*validationRules)
	}

	rules := &validationRules{relationFields: map[string]bool{}, validatedFields: map[string]bool{}}
	for _, name := range structinfo.RelationFieldNames(t, tagName) {
		rules.relationFields[name] = true
	}

	// regexp patterns are kept in a separate tag as they often contain characters
	// that have a special meaning in the `validate` tag, such as ',' and '|'
	regexpTagName := tagName + "_regexp"
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() || rules.relationFields[field.Name] {
			continue
		}

		if hasValidateTags(field, map[reflect.Type]bool{t: true}) {
			rules.validatedFields[field.Name] = true
		}

		validateTag := field.Tag.Get(validateTagName)
		pattern := field.Tag.Get(regexpTagName)
		if pattern == "" || validateTag == "-" || structinfo.BaseType(field.Type).Kind() != reflect.String {
			continue
		}
		rules.regexpFields = append(rules.regexpFields, regexpRule{
			index:     i,
			name:      field.Name,
			pattern:   pattern,
			omitempty: strings.Split(validateTag, ",")[0] == "omitempty",
		})
	}

	validationRuleCache.Store(key, rules)
	return rules
}

// fieldHasValidationRules checks if Validate can return a violation of the field of obj, a pointer to a struct.
func fieldHasValidationRules(obj any, name string, tagName string) bool {
	rules := getValidationRules(reflect.TypeOf(obj).Elem(), tagName)
	if rules.validatedFields[name] {
		return true
	}
	for _, rule := range rules.regexpFields {
		if rule.name == name {
			return true
		}
	}
	return false
}

// hasValidateTags checks if go-playground validates field: if it has a `validate` tag, or if it is a struct, which
// go-playground validates as well, with fields that have them. seen are the struct types that are already checked.
func hasValidateTags(field reflect.StructField, seen map[reflect.Type]bool) bool {
	tag := field.Tag.Get(validateTagName)
	if tag == "-" {
		return false
	}
	if tag != "" {
		return true
	}

	t := field.Type
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || structinfo.IsTimeType(t) || seen[t] {
		return false
	}

	seen[t] = true
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).IsExported() && hasValidateTags(t.Field(i), seen) {
			return true
		}
	}
	return false
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

func isNilValue(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}
