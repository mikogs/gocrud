package sqlite

import (
	"fmt"

	"github.com/mikogs/gocrud/pkg/filters"
	sqliteQueryContainer "github.com/mikogs/gocrud/pkg/sql/builder/querycontainer/sqlite"
	"github.com/mikogs/gocrud/pkg/structinfo"
)

type QueryBuilder struct {
	structInfo     *structinfo.StructInfo
	queryContainer *sqliteQueryContainer.QueryContainer
}

func New(structInfo *structinfo.StructInfo, queryContainer *sqliteQueryContainer.QueryContainer) *QueryBuilder {
	return &QueryBuilder{
		structInfo:     structInfo,
		queryContainer: queryContainer,
	}
}

func (b *QueryBuilder) Select(order []string, limit int, offset int, filters filters.Filter) (string, error) {
	query := b.queryContainer.SelectPrefix

	qOrder, err := b.queryOrder(order, b.queryContainer.CustomSelectPrefix)
	if err != nil {
		return "", getClauseBuilderError("order", "order array", err)
	}

	qLimitOffset := LimitOffset(limit, offset)
	qWhere, err := b.queryFilters(filters, b.queryContainer.CustomSelectPrefix)
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

func (b *QueryBuilder) SelectCount(filters filters.Filter) (string, error) {
	query := b.queryContainer.SelectCountPrefix

	qWhere, err := b.queryFilters(filters, b.queryContainer.CustomSelectCountPrefix)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}

func (b *QueryBuilder) Delete(filters filters.Filter) (string, error) {
	query := b.queryContainer.DeletePrefix

	qWhere, err := b.queryFilters(filters, false)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}

func (b *QueryBuilder) DeleteReturningID(filters filters.Filter) (string, error) {
	idFieldColumnName, ok := b.queryContainer.FieldColumnName["ID"]
	if !ok {
		return "", getClauseBuilderError("returning", "id field", idFieldNotFoundInQueryContainerError)
	}

	query := b.queryContainer.DeletePrefix

	qWhere, err := b.queryFilters(filters, false)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	query += fmt.Sprintf(` RETURNING "%s"`, sqliteQueryContainer.ColumnWithoutAlias(idFieldColumnName))

	return query, nil
}

func (b *QueryBuilder) Update(values map[string]interface{}, filters filters.Filter) (string, error) {
	query := b.queryContainer.UpdatePrefix

	qSet, err := b.querySet(values)
	if err != nil {
		return "", getClauseBuilderError("set", "values map", err)
	}

	query += " " + qSet

	qWhere, err := b.queryFilters(filters, false)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}
