package gocrud

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
	"github.com/mikogs/gocrud/internal/structinfo"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

// prepareFilters returns a filter ready for a query of obj: with the values converted from strings when convert is
// true, and with the UUIDs in their standard form, and validates it.
func (c *CRUD) prepareFilters(obj any, filter sqlfilters.Filter, convert bool) (sqlfilters.Filter, error) {
	targets := c.filterTargets(obj)

	if err := checkPasswordFilters(targets, filter); err != nil {
		return nil, err
	}

	var err error
	if convert {
		filter, err = convertFiltersFromString(targets, filter)
		if err != nil {
			return nil, err
		}
	}

	filter, err = c.prepareSubqueries(targets, filter)
	if err != nil {
		return nil, err
	}

	filter, err = normalizeUUIDFilters(targets, filter)
	if err != nil {
		return nil, err
	}

	filter, err = scanValuerFilters(targets, filter)
	if err != nil {
		return nil, err
	}

	return filter, validateFilters(targets, filter, c.tagName)
}

// checkPasswordFilters returns a validation error with the conditions of a filter on password fields, also through
// relation fields, e.g. Author.Password. A hash cannot be compared with a value, and conditions on it would reveal it,
// e.g. with LIKE.
func checkPasswordFilters(targets func(string) (*filterTarget, error), filter sqlfilters.Filter) error {
	violations := map[string]string{}
	for _, c := range sqlfilters.Conditions(filter) {
		target, err := targets(c.Field)
		if err != nil {
			return err
		}
		if target.password {
			violations[c.Field] = ViolationPassword
		}
	}

	if len(violations) > 0 {
		return getObjInvalidCRUDError(violations)
	}
	return nil
}

// convertFiltersFromString returns a filter with the string values of its conditions converted to the types of the
// fields. Raw filters, and IS NULL and IS NOT NULL conditions, which have a nil value, are not converted.
func convertFiltersFromString(targets func(string) (*filterTarget, error), filter sqlfilters.Filter) (sqlfilters.Filter, error) {
	return mapFilters(filter, func(filters sqlfilters.Filters) (sqlfilters.Filters, error) {
		return convertFilterValuesFromString(targets, filters)
	})
}

// convertFilterValuesFromString returns the conditions of a Filters map with their string values converted to the
// types of the fields. The values of OpIn are separated with commas.
func convertFilterValuesFromString(targets func(string) (*filterTarget, error), filters sqlfilters.Filters) (sqlfilters.Filters, error) {
	newFilters := sqlfilters.Filters{}
	for filterName, filterOpVal := range filters {
		if _, ok := filterOpVal.Val.(sqlfilters.SubqueryValue); ok || filterOpVal.IsNull() {
			newFilters.Add(filterName, filterOpVal)
			continue
		}

		target, err := targets(filterName)
		if err != nil {
			return nil, err
		}

		valueAsString, ok := filterOpVal.Val.(string)
		if !ok {
			return nil, getObjInvalidCRUDError(map[string]string{filterName: ViolationType})
		}

		// the values of IN are separated with commas, and an empty string is an empty list
		if filterOpVal.Op == sqlfilters.OpIn {
			values := []any{}
			if valueAsString != "" {
				for _, part := range strings.Split(valueAsString, ",") {
					ok, value := sqlbuilder.StructFieldValueFromString(target.obj, target.field, part)
					if !ok {
						return nil, getObjInvalidCRUDError(map[string]string{filterName: ViolationType})
					}
					values = append(values, value)
				}
			}

			newFilters.Add(filterName, sqlfilters.OpVal{Op: filterOpVal.Op, Val: values})
			continue
		}

		ok, valueAsFieldType := sqlbuilder.StructFieldValueFromString(target.obj, target.field, valueAsString)
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

// prepareSubqueries returns a filter with the filters of its subqueries prepared for the queries of their structs, and
// checks that the field of each subquery has values that can be compared with the values of the field of its
// condition.
func (c *CRUD) prepareSubqueries(targets func(string) (*filterTarget, error), filter sqlfilters.Filter) (sqlfilters.Filter, error) {
	return mapFilters(filter, func(filters sqlfilters.Filters) (sqlfilters.Filters, error) {
		// the filters are copied only when they are changed, and the filters of the callers are never changed
		prepared, cloned := filters, false
		for name, condition := range filters {
			subquery, ok := condition.Val.(sqlfilters.SubqueryValue)
			if !ok {
				continue
			}

			target, err := targets(name)
			if err != nil {
				return nil, err
			}

			subquery.Filter, err = c.prepareSubquery(target, condition.Op, subquery)
			if err != nil {
				return nil, getCRUDFuncCRUDError("filters", fmt.Errorf("%w: %s: %w", ErrSubquery, name, err))
			}
			if !cloned {
				prepared, cloned = maps.Clone(filters), true
			}
			prepared[name] = sqlfilters.OpVal{Op: condition.Op, Val: subquery}
		}
		return prepared, nil
	})
}

// prepareSubquery returns the filter of a subquery in a condition on the field of target, prepared for a query of the
// struct of the subquery.
func (c *CRUD) prepareSubquery(target *filterTarget, op int, subquery sqlfilters.SubqueryValue) (sqlfilters.Filter, error) {
	if op != sqlfilters.OpIn {
		return nil, errors.New("a subquery can only be the value of OpIn")
	}

	t := reflect.TypeOf(subquery.Obj)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%T is not a struct", subquery.Obj)
	}

	// a new object, as validation sets its fields
	obj := reflect.New(t).Interface()
	builder, err := c.builder(obj)
	if err != nil {
		return nil, err
	}

	if !slices.Contains(builder.ColumnFieldNames(), subquery.Field) {
		return nil, fmt.Errorf("%s is not a stored field of %s", subquery.Field, t.Name())
	}
	if slices.Contains(builder.PasswordFields(), subquery.Field) {
		return nil, fmt.Errorf("%s is a password field of %s", subquery.Field, t.Name())
	}

	field, _ := t.FieldByName(subquery.Field)
	if outer, ok := reflect.Indirect(reflect.ValueOf(target.obj)).Type().FieldByName(target.field); ok &&
		(target.json || valueKind(outer.Type) != valueKind(field.Type)) {
		return nil, fmt.Errorf("%s.%s (%s) cannot be compared with %s (%s)", t.Name(), subquery.Field, field.Type, target.field, outer.Type)
	}

	return c.prepareFilters(obj, subquery.Filter, false)
}

// valueKind returns the kind of the values of a field in the database, in which all numbers can be compared, e.g. of
// a decimal type, which is written as a string, "string". It is empty for the types that are only stored as JSON, e.g.
// maps.
func valueKind(t reflect.Type) string {
	goType := structinfo.ColumnGoType(t)
	switch {
	case strings.HasPrefix(goType, "int") || strings.HasPrefix(goType, "uint") || strings.HasPrefix(goType, "float"):
		return "number"
	default:
		return goType
	}
}

// normalizeUUIDFilters returns a filter with the UUIDs in the conditions on fields with UUIDs in their standard form,
// as SQLite compares them as text. A value that is not a UUID is a type violation. LIKE and regular expressions are
// not changed.
func normalizeUUIDFilters(targets func(string) (*filterTarget, error), filter sqlfilters.Filter) (sqlfilters.Filter, error) {
	return mapFilters(filter, func(filters sqlfilters.Filters) (sqlfilters.Filters, error) {
		normalized, cloned := filters, false
		var violations map[string]string
		for name, condition := range filters {
			target, err := targets(name)
			if err != nil {
				return nil, err
			}
			if _, ok := condition.Val.(sqlfilters.SubqueryValue); ok {
				continue
			}
			if !target.uuid || condition.IsNull() || condition.Op == sqlfilters.OpLike || condition.Op == sqlfilters.OpMatch {
				continue
			}

			value, err := uuidConditionValue(condition)
			if err != nil {
				if violations == nil {
					violations = map[string]string{}
				}
				violations[name] = ViolationType
				continue
			}
			if !cloned {
				normalized, cloned = maps.Clone(filters), true
			}
			normalized[name] = sqlfilters.OpVal{Op: condition.Op, Val: value}
		}

		if len(violations) > 0 {
			return nil, getObjInvalidCRUDError(violations)
		}
		return normalized, nil
	})
}

// uuidConditionValue returns the value of a condition with the UUIDs in their standard form: a list for OpIn.
func uuidConditionValue(condition sqlfilters.OpVal) (any, error) {
	if condition.Op != sqlfilters.OpIn {
		return idValue(true, condition.Val)
	}

	ids, ok := condition.InValues()
	if !ok {
		return nil, ErrInvalidID
	}
	for i, id := range ids {
		var err error
		if ids[i], err = idValue(true, id); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// scanValuerFilters returns a filter with the values of the conditions on fields of Valuer types of other types scanned
// into values of the types of the fields, e.g. a decimal from a string, so that they are written as the values of the
// fields are. A value that cannot be scanned is a type violation.
func scanValuerFilters(targets func(string) (*filterTarget, error), filter sqlfilters.Filter) (sqlfilters.Filter, error) {
	return mapFilters(filter, func(filters sqlfilters.Filters) (sqlfilters.Filters, error) {
		scanned, cloned := filters, false
		var violations map[string]string
		for name, condition := range filters {
			target, err := targets(name)
			if err != nil {
				return nil, err
			}
			if target.valuer == nil {
				continue
			}
			if _, ok := condition.Val.(sqlfilters.SubqueryValue); ok {
				continue
			}

			value, ok := valuerConditionValue(target.valuer, condition)
			if !ok {
				if violations == nil {
					violations = map[string]string{}
				}
				violations[name] = ViolationType
				continue
			}
			if !cloned {
				scanned, cloned = maps.Clone(filters), true
			}
			scanned[name] = sqlfilters.OpVal{Op: condition.Op, Val: value}
		}

		if len(violations) > 0 {
			return nil, getObjInvalidCRUDError(violations)
		}
		return scanned, nil
	})
}

// valuerConditionValue returns the value of a condition on a field of the Valuer type t as values of t: a list for
// OpIn.
func valuerConditionValue(t reflect.Type, condition sqlfilters.OpVal) (any, bool) {
	if condition.Op != sqlfilters.OpIn {
		return valuerValue(t, condition.Val)
	}

	values, ok := condition.InValues()
	if !ok {
		return condition.Val, true
	}
	for i, value := range values {
		if values[i], ok = valuerValue(t, value); !ok {
			return nil, false
		}
	}
	return values, true
}

// valuerValue returns value as a value of the Valuer type t, scanned as it would be from the database. A value of t,
// or a pointer to one, and nil are not changed.
func valuerValue(t reflect.Type, value any) (any, bool) {
	if isNilValue(value) {
		return value, true
	}
	if valueType := reflect.TypeOf(value); valueType == t || valueType == reflect.PointerTo(t) {
		return value, true
	}

	scanned, ok := structinfo.ScanValue(t, value)
	if !ok {
		return nil, false
	}
	return scanned.Interface(), true
}

// ValidateFilters validates the values of the conditions of a filter as if they were the values of the fields. A nil
// value is valid only for a nullable field, with OpEqual or OpNotEqual. The fields of related structs, e.g.
// Customer.Country, are validated with the tags of the related struct, and the filters of subqueries with the tags of
// their structs. A condition on a password field is a ViolationPassword. Raw filters are not validated.
func (c *CRUD) ValidateFilters(obj any, filter sqlfilters.Filter) error {
	targets := c.filterTargets(obj)
	if err := checkPasswordFilters(targets, filter); err != nil {
		return err
	}
	filter, err := c.prepareSubqueries(targets, filter)
	if err != nil {
		return err
	}
	return validateFilters(targets, filter, c.tagName)
}

// validateFilters returns a validation error with the violations of the conditions of filter, by their fields, or
// paths, e.g. Customer.Country.
func validateFilters(targets func(string) (*filterTarget, error), filter sqlfilters.Filter, tagName string) error {
	violations := map[string]string{}
	for _, c := range sqlfilters.Conditions(filter) {
		if _, ok := violations[c.Field]; ok {
			continue
		}

		target, err := targets(c.Field)
		if err != nil {
			return err
		}

		fieldViolations, err := validateCondition(target, c.OpVal, tagName)
		if err != nil {
			return err
		}
		// the violations are of the field of the target, which is the last part of the path
		if violation, ok := fieldViolations[target.field]; ok {
			violations[c.Field] = violation
		}
	}

	if len(violations) > 0 {
		return getObjInvalidCRUDError(violations)
	}

	return nil
}

// validateCondition returns the violations of the value of a condition on the field of target.
func validateCondition(target *filterTarget, condition sqlfilters.OpVal, tagName string) (map[string]string, error) {
	field, isField := reflect.Indirect(reflect.ValueOf(target.obj)).Type().FieldByName(target.field)

	// the filter of a subquery is validated with its struct
	if _, ok := condition.Val.(sqlfilters.SubqueryValue); ok {
		return nil, nil
	}

	// a value stored as JSON is only compared with NULL
	if target.json {
		if condition.IsNull() && field.Type.Kind() == reflect.Pointer {
			return nil, nil
		}
		return map[string]string{target.field: ViolationType}, nil
	}

	if condition.Op == sqlfilters.OpIn {
		return validateInFilter(target.obj, target.field, condition, tagName)
	}

	if condition.IsNull() {
		if isField && !structinfo.IsNullableType(field.Type) {
			return map[string]string{target.field: ViolationType}, nil
		}
		return nil, nil
	}

	// nil is valid only with OpEqual and OpNotEqual, as IS NULL and IS NOT NULL
	if isField && isNilValue(condition.Val) {
		return map[string]string{target.field: ViolationType}, nil
	}

	if err := sqlfilters.SetObjFields(target.obj, &sqlfilters.Filters{target.field: condition}); err != nil {
		return map[string]string{target.field: ViolationType}, nil
	}

	if !fieldHasValidationRules(target.obj, target.field, tagName) {
		return nil, nil
	}

	_, violations, err := Validate(target.obj, map[string]bool{target.field: true}, tagName)
	if err != nil {
		return nil, getValidateObjCRUDError(err)
	}
	return violations, nil
}

// validateInFilter validates each value of an OpIn filter as if it was the value of the field, and returns the
// violations of the first invalid value.
func validateInFilter(obj any, name string, filter sqlfilters.OpVal, tagName string) (map[string]string, error) {
	values, ok := filter.InValues()
	if !ok {
		return map[string]string{name: ViolationType}, nil
	}

	validated := fieldHasValidationRules(obj, name, tagName)
	for _, value := range values {
		// NULL is never IN a list
		if isNilValue(value) {
			return map[string]string{name: ViolationType}, nil
		}

		err := sqlfilters.SetObjFields(obj, &sqlfilters.Filters{name: {Op: sqlfilters.OpEqual, Val: value}})
		if err != nil {
			return map[string]string{name: ViolationType}, nil
		}
		if !validated {
			continue
		}

		ok, violations, err := Validate(obj, map[string]bool{name: true}, tagName)
		if err != nil {
			return nil, getValidateObjCRUDError(err)
		}

		if !ok {
			return violations, nil
		}
	}

	return nil, nil
}

// mapFilters returns a filter with each of the Filters maps in it replaced with the result of mapFunc.
func mapFilters(filter sqlfilters.Filter, mapFunc func(sqlfilters.Filters) (sqlfilters.Filters, error)) (sqlfilters.Filter, error) {
	switch f := filter.(type) {
	case *sqlfilters.Filters:
		if f == nil {
			return f, nil
		}
		mapped, err := mapFunc(*f)
		return &mapped, err
	case sqlfilters.Filters:
		return mapFunc(f)
	case sqlfilters.Group:
		mapped := sqlfilters.Group{Or: f.Or, Filters: make([]sqlfilters.Filter, len(f.Filters))}
		for i, child := range f.Filters {
			var err error
			mapped.Filters[i], err = mapFilters(child, mapFunc)
			if err != nil {
				return nil, err
			}
		}
		return mapped, nil
	default:
		return filter, nil
	}
}

// filterTarget is the struct and its field that a field of a filter refers to: obj itself for a field of obj, and a
// related struct for a path through relation fields, e.g. Customer.Country.
type filterTarget struct {
	obj      any
	field    string
	uuid     bool
	password bool
	json     bool
	// valuer is the Valuer type of the field, or of what it points to, or nil
	valuer reflect.Type
}

// filterTargets returns a function that returns the target of a field of a filter of obj. The objects of related
// structs are created once, as validation sets their fields.
func (c *CRUD) filterTargets(obj any) func(string) (*filterTarget, error) {
	targets := map[string]*filterTarget{}

	return func(path string) (*filterTarget, error) {
		if target, ok := targets[path]; ok {
			return target, nil
		}

		segments := strings.Split(path, ".")
		current := obj
		for i, segment := range segments[:len(segments)-1] {
			builder, err := c.builder(current)
			if err != nil {
				return nil, getBuilderObjectCRUDError(err)
			}

			var relation *structinfo.RelationInfo
			for _, r := range builder.Relations() {
				if r.Field == segment {
					relation = r
				}
			}
			if relation == nil {
				return nil, getCRUDFuncCRUDError("filters", fmt.Errorf("%w: %s: %s is not a relation field of %s",
					ErrFilterPath, path, strings.Join(segments[:i+1], "."), reflect.Indirect(reflect.ValueOf(current)).Type().Name()))
			}

			if parent, ok := targets[strings.Join(segments[:i+1], ".")]; ok {
				current = parent.obj
			} else {
				current = reflect.New(relation.Type).Interface()
				targets[strings.Join(segments[:i+1], ".")] = &filterTarget{obj: current}
			}
		}

		builder, err := c.builder(current)
		if err != nil {
			return nil, getBuilderObjectCRUDError(err)
		}

		field := segments[len(segments)-1]
		target := &filterTarget{obj: current, field: field}
		for _, uuidField := range builder.UUIDFields() {
			target.uuid = target.uuid || uuidField == field
		}
		target.password = slices.Contains(builder.PasswordFields(), field)
		target.json = slices.Contains(builder.JSONFields(), field)
		target.valuer = builder.ValuerType(field)
		targets[path] = target
		return target, nil
	}
}
