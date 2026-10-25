package sqlbuilder

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mikogs/gocrud/internal/sqlbuilder/where"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

// The queries of a struct that depend on filters, orders and values, e.g. SELECT ... WHERE. The values of their
// placeholders are in the order of sqlfilters.FiltersInterfaces.

func (b *Builder) selectQuery(order []string, limit int, offset int, filters sqlfilters.Filter) (string, error) {
	query := b.st.SelectPrefix

	qOrder, err := b.queryOrder(order, b.st.CustomSelectPrefix)
	if err != nil {
		return "", getClauseBuilderError("order", "order array", err)
	}

	qLimitOffset := limitOffset(limit, offset)
	qWhere, err := b.queryFilters(filters, 1, b.st.CustomSelectPrefix)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}
	if qOrder != "" {
		query += " ORDER BY " + qOrder
	}
	if qLimitOffset != "" {
		query += " " + qLimitOffset
	}

	return query, nil
}

func (b *Builder) selectCountQuery(filters sqlfilters.Filter) (string, error) {
	query := b.st.SelectCountPrefix

	qWhere, err := b.queryFilters(filters, 1, b.st.CustomSelectCountPrefix)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}

func (b *Builder) deleteQuery(filters sqlfilters.Filter) (string, error) {
	query := b.st.DeletePrefix

	qWhere, err := b.queryFilters(filters, 1, false)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}

func (b *Builder) deleteReturningIDQuery(filters sqlfilters.Filter) (string, error) {
	idFieldColumnName, ok := b.st.FieldColumnName["ID"]
	if !ok {
		return "", getClauseBuilderError("returning", "id field", errIDFieldNotFoundInQueryContainer)
	}

	idColumn := columnWithoutAlias(idFieldColumnName)

	query := b.st.DeletePrefix

	qWhere, err := b.queryFilters(filters, 1, false)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	query += fmt.Sprintf(` RETURNING "%s"`, idColumn)

	return query, nil
}

func (b *Builder) updateQuery(values map[string]any, filters sqlfilters.Filter) (string, error) {
	query := b.st.UpdatePrefix

	qSet, lastVarNumber, err := b.querySet(values)
	if err != nil {
		return "", getClauseBuilderError("set", "values map", err)
	}

	query += " " + qSet

	qWhere, err := b.queryFilters(filters, lastVarNumber+1, false)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}

func (b *Builder) queryOrder(order []string, withAlias bool) (string, error) {
	if len(order) == 0 {
		return "", nil
	}

	if len(order)%2 != 0 {
		return "", getOrderBuilderError()
	}

	qOrder := ""
	for i := 0; i < len(order); i = i + 2 {
		field := order[i]
		direction := order[i+1]

		queryDirection := "ASC"
		if strings.EqualFold(direction, "desc") {
			queryDirection = "DESC"
		}

		// a field of a related struct, e.g. Customer.Name, is selected by a subquery, which has no values, and is NULL when
		// there is no related row, which comes last in both directions in all the databases
		if strings.Contains(field, ".") {
			column, err := where.OrderColumn(field, b.whereDialect("", withAlias, func() string { return "" }))
			if err != nil {
				return "", err
			}
			qOrder += fmt.Sprintf(`,%s %s NULLS LAST`, column, queryDirection)
			continue
		}

		columnName, ok := b.st.FieldColumnName[field]
		if !ok {
			return "", getColumnNameBuilderError("order")
		}
		if b.structInfo.Fields[field].Password {
			return "", fmt.Errorf("%w: %s", where.ErrPasswordField, field)
		}

		qOrder += fmt.Sprintf(`,%s %s`, quoteColumn(columnName, withAlias), queryDirection)
	}

	if qOrder == "" {
		return qOrder, nil
	}

	return qOrder[1:], nil
}

func (b *Builder) querySet(values map[string]any) (string, int, error) {
	if len(values) == 0 {
		return "", 0, nil
	}

	columns := make([]string, 0, len(values))
	for value := range values {
		fieldInfo, ok := b.structInfo.Fields[value]
		if !ok {
			return "", 0, getFieldInfosIsNilError()
		}
		if fieldInfo.Ignored {
			return "", 0, getFieldIsIgnoredError()
		}

		columnName, ok := b.st.FieldColumnName[value]
		if !ok {
			return "", 0, getColumnNameBuilderError("value")
		}

		columns = append(columns, columnName)
	}

	numColumn := len(columns)
	if numColumn == 0 {
		return "", 0, nil
	}

	sort.Strings(columns)

	querySet := ""
	for i := 1; i <= numColumn; i++ {
		querySet += fmt.Sprintf(`,%s=%s`, quoteName(columnWithoutAlias(columns[i-1])), b.dialect.Placeholder(i))
	}
	// the rows are changed, so that the objects loaded before cannot be saved without loading them again
	if b.st.VersionColumn != "" {
		querySet += fmt.Sprintf(`,%s=%s+1`, b.st.VersionColumn, b.st.VersionColumn)
	}

	return querySet[1:], numColumn, nil
}

func (b *Builder) queryFilters(f sqlfilters.Filter, firstValueNum int, withAlias bool) (string, error) {
	valueNum := firstValueNum
	placeholder := func() string {
		valueNum++
		return b.dialect.Placeholder(valueNum - 1)
	}

	return where.Build(f, b.whereDialect("", withAlias, placeholder))
}

// WhereDialect returns the dialect of the conditions on the fields of the struct. The columns are qualified with
// qualifier, e.g. the alias of the table in a subquery, unless it is empty.
func (b *Builder) whereDialect(qualifier string, withAlias bool, placeholder func() string) where.Dialect {
	d := where.Dialect{
		Column: func(field string) (string, error) {
			column, err := b.filterColumn(field, withAlias)
			if err != nil || qualifier == "" {
				return column, err
			}
			return qualifier + "." + column, nil
		},
		TextColumn: func(field string, column string) string {
			return b.dialect.TextColumn(column, b.st.ColumnNotString[b.st.FieldColumnName[field]])
		},
		Placeholder:   placeholder,
		MatchOperator: b.dialect.MatchOperator(),
	}

	outer := qualifier
	if outer == "" {
		outer = b.st.TableName
	}
	d.Relation = func(field string, alias string) (where.Related, error) {
		if withAlias {
			return where.Related{}, fmt.Errorf("%w: %s: the struct has its own select query", where.ErrRelationPath, field)
		}
		return b.relation(outer, field, alias, placeholder)
	}
	d.Subquery = func(obj any, field string, alias string) (where.Selected, error) {
		return b.subquery(obj, field, alias, placeholder)
	}

	return d
}

// filterColumn returns the quoted column of a field that is used in a filter.
func (b *Builder) filterColumn(field string, withAlias bool) (string, error) {
	fieldInfo, ok := b.structInfo.Fields[field]
	if !ok {
		return "", getFieldInfosIsNilError()
	}
	if fieldInfo.Ignored {
		return "", getFieldIsIgnoredError()
	}
	if fieldInfo.Password {
		return "", fmt.Errorf("%w: %s", where.ErrPasswordField, field)
	}

	columnName, ok := b.st.FieldColumnName[field]
	if !ok {
		return "", getColumnNameBuilderError("filter")
	}

	return quoteColumn(columnName, withAlias), nil
}

func limitOffset(limit int, offset int) string {
	if limit == 0 {
		return ""
	}

	if offset > 0 {
		return fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
	}

	return fmt.Sprintf("LIMIT %d", limit)
}

type QueryBuilderError struct {
	Op  string
	Tag string
	Err error
}

func (e *QueryBuilderError) Error() string {
	return e.Op + ": " + e.Err.Error()
}

func (e *QueryBuilderError) Unwrap() error {
	return e.Err
}

var errIDFieldNotFoundInQueryContainer = errors.New("id field not found in query container")
var errFieldNameNotFound = errors.New("field name not found")
var errFieldIsIgnored = errors.New("field is ignored")
var errFieldInfosIsNil = errors.New("field infos is nil")

var getClauseBuilderError = func(clause, source string, err error) *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get " + clause + " clause from " + source,
		Err: err,
	}
}

var getColumnNameBuilderError = func(source string) *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get column name from " + source + " field",
		Err: errFieldNameNotFound,
	}
}

var getFieldIsIgnoredError = func() *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get field infos",
		Err: errFieldIsIgnored,
	}
}

var getFieldInfosIsNilError = func() *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get field infos",
		Err: errFieldInfosIsNil,
	}
}

var errOrder = errors.New("order slice must have an even number of elements (field, direction pairs)")

var getOrderBuilderError = func() *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get order",
		Err: errOrder,
	}
}
