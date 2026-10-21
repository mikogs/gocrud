package postgres

import (
	"fmt"
	"regexp"

	"github.com/mikogs/gocrud/pkg/filters"
	postgresQueryContainer "github.com/mikogs/gocrud/pkg/sql/builder/querycontainer/postgres"
	"github.com/mikogs/gocrud/pkg/structinfo"
)

var (
	regexpFieldInRaw = regexp.MustCompile(`\.[a-zA-Z0-9_]+`)
)

type QueryBuilder struct {
	structInfo     *structinfo.StructInfo
	queryContainer *postgresQueryContainer.QueryContainer
}

func New(structInfo *structinfo.StructInfo, queryContainer *postgresQueryContainer.QueryContainer) *QueryBuilder {
	queryBuilder := &QueryBuilder{
		structInfo:     structInfo,
		queryContainer: queryContainer,
	}
	return queryBuilder
}

// Select returns a SELECT query with a WHERE condition built from 'filters' (field-value pairs).
// Struct fields in the 'filters' argument are sorted alphabetically. Hence, when used with a database connection, their values (or pointers to it) must be sorted as well.
// Columns in the SELECT query are ordered the same way as they are defined in the struct: SELECT field1_column, field2_column, ... etc.
func (b *QueryBuilder) Select(order []string, limit int, offset int, filters *filters.Filters) (string, error) {
	query := b.queryContainer.SelectPrefix

	qOrder, err := b.queryOrder(order, b.queryContainer.CustomSelectPrefix)
	if err != nil {
		return "", getClauseBuilderError("order", "order array", err)
	}

	qLimitOffset := LimitOffset(limit, offset)
	qWhere, err := b.queryFilters(filters, 1, b.queryContainer.CustomSelectPrefix)
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

// SelectCount returns a SELECT COUNT(*) query to count rows with a WHERE condition built from 'filters' (field-value pairs).
// Struct fields in the 'filters' argument are sorted alphabetically. Hence, when used with a database connection, their values (or pointers to it) must be sorted as well.
func (b *QueryBuilder) SelectCount(filters *filters.Filters) (string, error) {
	query := b.queryContainer.SelectCountPrefix

	qWhere, err := b.queryFilters(filters, 1, b.queryContainer.CustomSelectCountPrefix)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}

// Delete returns a DELETE query with a WHERE condition built from 'filters' (field-value pairs).
// Struct fields in the 'filters' argument are sorted alphabetically. Hence, when used with a database connection, their values (or pointers to it) must be sorted as well.
func (b *QueryBuilder) Delete(filters *filters.Filters) (string, error) {
	query := b.queryContainer.DeletePrefix

	qWhere, err := b.queryFilters(filters, 1, false)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}

// DeleteReturningID returns a DELETE query with a WHERE condition built from 'filters' (field-value pairs) with RETURNING id.
// Struct fields in the 'filters' argument are sorted alphabetically. Hence, when used with a database connection, their values (or pointers to it) must be sorted as well.
func (b *QueryBuilder) DeleteReturningID(filters *filters.Filters) (string, error) {
	idFieldColumnName, ok := b.queryContainer.FieldColumnName["ID"]
	if !ok {
		return "", getClauseBuilderError("returning", "id field", idFieldNotFoundInQueryContainerError)
	}

	idColumn := postgresQueryContainer.ColumnWithoutAlias(idFieldColumnName)

	query := b.queryContainer.DeletePrefix

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

// Update returns an UPDATE query where specified struct fields (columns) are updated and rows match specific WHERE condition built from 'filters' (field-value pairs).
// Struct fields in 'values' and the 'filters' arguments are sorted alphabetically. Hence, when used with a database connection, their values (or pointers to it) must be sorted as well.
func (b *QueryBuilder) Update(values map[string]interface{}, filters *filters.Filters) (string, error) {
	query := b.queryContainer.UpdatePrefix

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
