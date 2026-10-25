// Package where builds the conditions of a WHERE clause from filters, for any database.
package where

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

// regexpFieldInRaw matches a field in a raw filter, e.g. .Name, but not a column of a table, e.g. t1.name, or a number.
var regexpFieldInRaw = regexp.MustCompile(`(^|[^a-zA-Z0-9_"])\.([a-zA-Z_][a-zA-Z0-9_]*)`)

var (
	ErrInFilter     = errors.New("value of an OpIn filter must be a slice or an array")
	ErrMatchFilter  = errors.New("OpMatch is not supported")
	ErrRawFilter    = errors.New("invalid raw filter")
	ErrRelationPath = errors.New("invalid relation path")
	ErrSubquery     = errors.New("invalid subquery")
	// ErrPasswordField is matched by the error of a condition or an order on a password field, whose hashes it would
	// reveal.
	ErrPasswordField = errors.New("password fields cannot be used in conditions or orders")
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
	// Relation returns the related rows of a relation field, with alias as the name of their table. When it is nil,
	// fields of related structs cannot be used.
	Relation func(field string, alias string) (Related, error)
	// Subquery returns the rows of the struct of obj for a subquery, with alias as the name of their table. When it is
	// nil, subqueries cannot be used.
	Subquery func(obj any, field string, alias string) (Selected, error)
}

// Related describes the related rows of a relation field.
type Related struct {
	// Dialect is the dialect of the related struct, with its columns qualified with the alias, and the same
	// Placeholder.
	Dialect Dialect
	// Table is the quoted name of the table of the related struct.
	Table string
	// Match is the condition that matches the related rows to a row of the struct of the relation field.
	Match string
	// Many is set to true for a has-many relation, which can have many related rows.
	Many bool
}

// Selected describes the rows of a subquery.
type Selected struct {
	// Dialect is the dialect of the struct of the subquery, with its columns qualified with the alias, and the same
	// Placeholder.
	Dialect Dialect
	// Table is the quoted name of the table of the struct.
	Table string
	// Column is the column of the field that the subquery selects, qualified with the alias.
	Column string
}

// state is shared by all the conditions of a query, including the conditions of its subqueries.
type state struct {
	aliases    int
	subqueries int
}

// Build returns the conditions of a WHERE clause, or an empty string when the filter has no conditions. Placeholders
// are in the order of the values that sqlfilters.FiltersInterfaces returns.
//
// A field can be a path through relation fields, e.g. Customer.Country. The conditions of a Filters map whose paths
// start with the same relation field are in one EXISTS subquery, so that they match the same related row.
//
// The value of an OpIn condition can be a sqlfilters.SubqueryValue, which is an IN subquery.
func Build(f sqlfilters.Filter, d Dialect) (string, error) {
	sql, _, err := build(f, d, &state{})
	return sql, err
}

// build returns the SQL of a filter, which is empty when it matches every row, and whether it has to be in parentheses
// when it is joined with other conditions.
func build(f sqlfilters.Filter, d Dialect, st *state) (string, bool, error) {
	switch f := f.(type) {
	case *sqlfilters.Filters:
		if f == nil {
			return "", false, nil
		}
		return build(*f, d, st)
	case sqlfilters.Filters:
		parts, err := conditions(sqlfilters.Conditions(f), d, st)
		if err != nil {
			return "", false, err
		}
		return strings.Join(parts, " AND "), len(parts) > 1, nil
	case sqlfilters.Group:
		return group(f, d, st)
	case sqlfilters.RawFilter:
		sql, err := raw(f, d)
		return sql, true, err
	case nil:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("unsupported filter %T", f)
	}
}

// conditions returns the SQL of the conditions of a Filters map, in their order. The conditions on fields of a
// relation are next to each other in the order, which is sorted by field name, so they are replaced with one EXISTS
// subquery where they are, and the placeholders stay in the order of the values.
func conditions(conds []sqlfilters.Condition, d Dialect, st *state) ([]string, error) {
	parts := []string{}
	for i := 0; i < len(conds); {
		relation, _, isPath := strings.Cut(conds[i].Field, ".")
		if !isPath {
			part, err := condition(conds[i], d, st)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
			i++
			continue
		}

		related := sqlfilters.Filters{}
		for ; i < len(conds); i++ {
			name, field, ok := strings.Cut(conds[i].Field, ".")
			if !ok || name != relation {
				break
			}
			related[field] = conds[i].OpVal
		}

		part, err := exists(relation, related, d, st)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}

	return parts, nil
}

// exists returns an EXISTS subquery that matches the rows of a relation for which the conditions are true.
func exists(relation string, conds sqlfilters.Filters, d Dialect, st *state) (string, error) {
	if d.Relation == nil {
		return "", fmt.Errorf("%w: %s: fields of related structs cannot be used here", ErrRelationPath, relation)
	}

	st.aliases++
	alias := fmt.Sprintf("r%d", st.aliases)

	related, err := d.Relation(relation, alias)
	if err != nil {
		return "", err
	}

	parts, err := conditions(sqlfilters.Conditions(conds), related.Dialect, st)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("EXISTS (SELECT 1 FROM %s %s WHERE %s AND %s)", related.Table, alias, related.Match, strings.Join(parts, " AND ")), nil
}

// OrderColumn returns the column of a field for ORDER BY. For a path through belongs-to relations, e.g. Customer.Name,
// it returns a subquery that selects the column of the related row, which is NULL when there is no related row.
func OrderColumn(field string, d Dialect) (string, error) {
	return orderColumn(field, d, 1)
}

func orderColumn(field string, d Dialect, depth int) (string, error) {
	relation, rest, isPath := strings.Cut(field, ".")
	if !isPath {
		return d.Column(field)
	}

	if d.Relation == nil {
		return "", fmt.Errorf("%w: %s: fields of related structs cannot be used here", ErrRelationPath, relation)
	}

	alias := fmt.Sprintf("r%d", depth)
	related, err := d.Relation(relation, alias)
	if err != nil {
		return "", err
	}
	if related.Many {
		return "", fmt.Errorf("%w: %s: a has-many relation has many values, and cannot be used to order", ErrRelationPath, relation)
	}

	column, err := orderColumn(rest, related.Dialect, depth+1)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("(SELECT %s FROM %s %s WHERE %s)", column, related.Table, alias, related.Match), nil
}

func group(g sqlfilters.Group, d Dialect, st *state) (string, bool, error) {
	parts := []string{}
	compound := []bool{}
	for _, filter := range g.Filters {
		part, isCompound, err := build(filter, d, st)
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

func condition(c sqlfilters.Condition, d Dialect, st *state) (string, error) {
	column, err := d.Column(c.Field)
	if err != nil {
		return "", err
	}

	if s, ok := c.Val.(sqlfilters.SubqueryValue); ok {
		if c.Op != sqlfilters.OpIn {
			return "", fmt.Errorf("%w: %s: a subquery can only be the value of OpIn", ErrSubquery, c.Field)
		}
		return subquery(column, s, d, st)
	}

	if c.IsNull() {
		if c.Op == sqlfilters.OpNotEqual {
			return column + " IS NOT NULL", nil
		}
		return column + " IS NULL", nil
	}

	switch c.Op {
	case sqlfilters.OpIn:
		values, ok := c.InValues()
		if !ok {
			return "", ErrInFilter
		}
		// an empty list matches nothing
		if len(values) == 0 {
			return "1=0", nil
		}
		return column + " IN (" + placeholders(len(values), d) + ")", nil
	case sqlfilters.OpLike:
		return d.TextColumn(c.Field, column) + " LIKE " + d.Placeholder(), nil
	case sqlfilters.OpMatch:
		if d.MatchOperator == "" {
			return "", ErrMatchFilter
		}
		return d.TextColumn(c.Field, column) + " " + d.MatchOperator + " " + d.Placeholder(), nil
	case sqlfilters.OpNotEqual:
		return column + "!=" + d.Placeholder(), nil
	case sqlfilters.OpGreater:
		return column + ">" + d.Placeholder(), nil
	case sqlfilters.OpLower:
		return column + "<" + d.Placeholder(), nil
	case sqlfilters.OpGreaterOrEqual:
		return column + ">=" + d.Placeholder(), nil
	case sqlfilters.OpLowerOrEqual:
		return column + "<=" + d.Placeholder(), nil
	case sqlfilters.OpBit:
		return column + "&" + d.Placeholder() + ">0", nil
	default:
		return column + "=" + d.Placeholder(), nil
	}
}

// subquery returns an IN condition with a subquery that selects the values of a field of the rows that match a filter.
func subquery(column string, s sqlfilters.SubqueryValue, d Dialect, st *state) (string, error) {
	if d.Subquery == nil {
		return "", fmt.Errorf("%w: subqueries cannot be used here", ErrSubquery)
	}

	st.subqueries++
	alias := fmt.Sprintf("s%d", st.subqueries)

	selected, err := d.Subquery(s.Obj, s.Field, alias)
	if err != nil {
		return "", err
	}

	where, _, err := build(s.Filter, selected.Dialect, st)
	if err != nil {
		return "", err
	}

	sql := fmt.Sprintf("SELECT %s FROM %s %s", selected.Column, selected.Table, alias)
	if where != "" {
		sql += " WHERE " + where
	}
	return column + " IN (" + sql + ")", nil
}

// raw returns the SQL of a raw filter, with the fields replaced with their columns, and each ? replaced with a
// placeholder, or with a list of placeholders for a slice or an array.
func raw(r sqlfilters.RawFilter, d Dialect) (string, error) {
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
		return "", fmt.Errorf("%w: %d placeholders and %d values in %q", ErrRawFilter, len(parts)-1, len(r.Values), r.SQL)
	}

	var b strings.Builder
	b.WriteString(parts[0])
	for i, value := range r.Values {
		if values, ok := (sqlfilters.OpVal{Val: value}).InValues(); ok {
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
