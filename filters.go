package gocrud

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
	"github.com/mikogs/gocrud/internal/structinfo"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

// The filters of the operations are prepared for the queries by prepareFilters: their values are converted from
// strings, the UUIDs in them are in their standard form, the filters of their subqueries are prepared for the structs
// of the subqueries, and they are validated with the tags of the fields that they are on.

// prepareFilters returns a filter ready for a query of obj: with the values converted from strings when convert is
// true, and with the UUIDs in their standard form, and validates it.
func (c *CRUD) prepareFilters(obj interface{}, filter sqlfilters.Filter, convert bool) (sqlfilters.Filter, error) {
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
			values := []interface{}{}
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
		prepared := sqlfilters.Filters{}
		for name, condition := range filters {
			prepared[name] = condition

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
		valueKind(outer.Type) != valueKind(field.Type) {
		return nil, fmt.Errorf("%s.%s (%s) cannot be compared with %s (%s)", t.Name(), subquery.Field, field.Type, target.field, outer.Type)
	}

	return c.prepareFilters(obj, subquery.Filter, false)
}

// valueKind returns the kind of the values of a field in the database, in which all numbers can be compared.
func valueKind(t reflect.Type) string {
	t = structinfo.BaseType(t)
	switch {
	case structinfo.IsTimeType(t):
		return "time"
	case t.Kind() >= reflect.Int && t.Kind() <= reflect.Float64:
		return "number"
	default:
		return t.Kind().String()
	}
}

// normalizeUUIDFilters returns a filter with the UUIDs in the conditions on fields with UUIDs in their standard form,
// as SQLite compares them as text. A value that is not a UUID is a type violation. LIKE and regular expressions are
// not changed.
func normalizeUUIDFilters(targets func(string) (*filterTarget, error), filter sqlfilters.Filter) (sqlfilters.Filter, error) {
	return mapFilters(filter, func(filters sqlfilters.Filters) (sqlfilters.Filters, error) {
		normalized := sqlfilters.Filters{}
		violations := map[string]string{}
		for name, condition := range filters {
			normalized[name] = condition

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
				violations[name] = ViolationType
				continue
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
func uuidConditionValue(condition sqlfilters.OpVal) (interface{}, error) {
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

// ValidateFilters validates the values of the conditions of a filter as if they were the values of the fields. A nil
// value is valid only for a nullable field, with OpEqual or OpNotEqual. The fields of related structs, e.g.
// Customer.Country, are validated with the tags of the related struct, and the filters of subqueries with the tags of
// their structs. A condition on a password field is a ViolationPassword. Raw filters are not validated.
func (c *CRUD) ValidateFilters(obj interface{}, filter sqlfilters.Filter) error {
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

	_, violations, err := Validate(target.obj, map[string]bool{target.field: true}, tagName)
	if err != nil {
		return nil, getValidateObjCRUDError(err)
	}
	return violations, nil
}

// validateInFilter validates each value of an OpIn filter as if it was the value of the field, and returns the
// violations of the first invalid value.
func validateInFilter(obj interface{}, name string, filter sqlfilters.OpVal, tagName string) (map[string]string, error) {
	values, ok := filter.InValues()
	if !ok {
		return map[string]string{name: ViolationType}, nil
	}

	for _, value := range values {
		// NULL is never IN a list
		if isNilValue(value) {
			return map[string]string{name: ViolationType}, nil
		}

		err := sqlfilters.SetObjFields(obj, &sqlfilters.Filters{name: {Op: sqlfilters.OpEqual, Val: value}})
		if err != nil {
			return map[string]string{name: ViolationType}, nil
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
	obj      interface{}
	field    string
	uuid     bool
	password bool
}

// filterTargets returns a function that returns the target of a field of a filter of obj. The objects of related
// structs are created once, as validation sets their fields.
func (c *CRUD) filterTargets(obj interface{}) func(string) (*filterTarget, error) {
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
		targets[path] = target
		return target, nil
	}
}
