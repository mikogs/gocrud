// Package where builds the conditions of a WHERE clause from filters, for any database.
package where

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/mikogs/gocrud/pkg/filters"
)

// regexpFieldInRaw matches a field in a raw filter, e.g. .Name, but not a column of a table, e.g. t1.name, or a number.
var regexpFieldInRaw = regexp.MustCompile(`(^|[^a-zA-Z0-9_"])\.([a-zA-Z_][a-zA-Z0-9_]*)`)

var (
	InFilterError    = errors.New("value of an OpIn filter must be a slice or an array")
	MatchFilterError = errors.New("OpMatch is not supported")
	RawFilterError   = errors.New("invalid raw filter")
)

// Dialect is what differs between databases in a WHERE clause.
type Dialect struct {
	// Column returns the quoted column of a field, or an error when the field cannot be used in a condition.
	Column func(field string) (string, error)
	// TextColumn returns the column of a field as text, for OpLike and OpMatch.
	TextColumn func(field string, column string) string
	// Placeholder returns the placeholder of the next value, e.g. $1 or ?.
	Placeholder func() string
	// MatchOperator is the operator of OpMatch, or empty when it is not supported.
	MatchOperator string
}

// Build returns the conditions of a WHERE clause, or an empty string when the filter has no conditions. Placeholders
// are in the order of the values that filters.FiltersInterfaces returns.
func Build(f filters.Filter, d Dialect) (string, error) {
	sql, _, err := build(f, d)
	return sql, err
}

// build returns the SQL of a filter, which is empty when it matches every row, and whether it has to be in parentheses
// when it is joined with other conditions.
func build(f filters.Filter, d Dialect) (string, bool, error) {
	switch f := f.(type) {
	case *filters.Filters:
		if f == nil {
			return "", false, nil
		}
		return build(*f, d)
	case filters.Filters:
		parts := []string{}
		for _, c := range filters.Conditions(f) {
			part, err := condition(c, d)
			if err != nil {
				return "", false, err
			}
			parts = append(parts, part)
		}
		return strings.Join(parts, " AND "), len(parts) > 1, nil
	case filters.Group:
		return group(f, d)
	case filters.RawFilter:
		sql, err := raw(f, d)
		return sql, true, err
	case nil:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("unsupported filter %T", f)
	}
}

func group(g filters.Group, d Dialect) (string, bool, error) {
	parts := []string{}
	compound := []bool{}
	for _, filter := range g.Filters {
		part, isCompound, err := build(filter, d)
		if err != nil {
			return "", false, err
		}

		// a filter that matches every row is left out of AND, and makes OR match every row
		if part == "" {
			if !g.Or {
				continue
			}
			part = "1=1"
		}

		parts = append(parts, part)
		compound = append(compound, isCompound)
	}

	if len(parts) == 0 {
		if g.Or {
			return "1=0", false, nil
		}
		return "", false, nil
	}

	if len(parts) == 1 {
		return parts[0], compound[0], nil
	}

	for i := range parts {
		if compound[i] {
			parts[i] = "(" + parts[i] + ")"
		}
	}

	joiner := " AND "
	if g.Or {
		joiner = " OR "
	}
	return strings.Join(parts, joiner), true, nil
}

func condition(c filters.Condition, d Dialect) (string, error) {
	column, err := d.Column(c.Field)
	if err != nil {
		return "", err
	}

	if c.IsNull() {
		if c.Op == filters.OpNotEqual {
			return column + " IS NOT NULL", nil
		}
		return column + " IS NULL", nil
	}

	switch c.Op {
	case filters.OpIn:
		values, ok := c.InValues()
		if !ok {
			return "", InFilterError
		}
		// an empty list matches nothing
		if len(values) == 0 {
			return "1=0", nil
		}
		return column + " IN (" + placeholders(len(values), d) + ")", nil
	case filters.OpLike:
		return d.TextColumn(c.Field, column) + " LIKE " + d.Placeholder(), nil
	case filters.OpMatch:
		if d.MatchOperator == "" {
			return "", MatchFilterError
		}
		return d.TextColumn(c.Field, column) + " " + d.MatchOperator + " " + d.Placeholder(), nil
	case filters.OpNotEqual:
		return column + "!=" + d.Placeholder(), nil
	case filters.OpGreater:
		return column + ">" + d.Placeholder(), nil
	case filters.OpLower:
		return column + "<" + d.Placeholder(), nil
	case filters.OpGreaterOrEqual:
		return column + ">=" + d.Placeholder(), nil
	case filters.OpLowerOrEqual:
		return column + "<=" + d.Placeholder(), nil
	case filters.OpBit:
		return column + "&" + d.Placeholder() + ">0", nil
	default:
		return column + "=" + d.Placeholder(), nil
	}
}

// raw returns the SQL of a raw filter, with the fields replaced with their columns, and each ? replaced with a
// placeholder, or with a list of placeholders for a slice or an array.
func raw(r filters.RawFilter, d Dialect) (string, error) {
	var columnErr error
	sql := regexpFieldInRaw.ReplaceAllStringFunc(r.SQL, func(match string) string {
		submatches := regexpFieldInRaw.FindStringSubmatch(match)
		column, err := d.Column(submatches[2])
		if err != nil && columnErr == nil {
			columnErr = err
		}
		return submatches[1] + column
	})
	if columnErr != nil {
		return "", columnErr
	}

	parts := strings.Split(sql, "?")
	if len(parts)-1 != len(r.Values) {
		return "", fmt.Errorf("%w: %d placeholders and %d values in %q", RawFilterError, len(parts)-1, len(r.Values), r.SQL)
	}

	var b strings.Builder
	b.WriteString(parts[0])
	for i, value := range r.Values {
		if values, ok := (filters.OpVal{Val: value}).InValues(); ok {
			b.WriteString(placeholders(len(values), d))
		} else {
			b.WriteString(d.Placeholder())
		}
		b.WriteString(parts[i+1])
	}

	return b.String(), nil
}

func placeholders(num int, d Dialect) string {
	list := make([]string, num)
	for i := range list {
		list[i] = d.Placeholder()
	}
	return strings.Join(list, ",")
}
