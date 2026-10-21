package sqlite

import (
	"fmt"
	"regexp"

	"github.com/mikogs/gocrud/pkg/filters"
	sqliteQueryContainer "github.com/mikogs/gocrud/pkg/sql/builder/querycontainer/sqlite"
	"github.com/mikogs/gocrud/pkg/structinfo"
)

var regexpFieldInRaw = regexp.MustCompile(`\.[a-zA-Z0-9_]+`)

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

func (b *QueryBuilder) Select(order []string, limit int, offset int, filters *filters.Filters) (string, error) {
	query := b.queryContainer.SelectPrefix

	qOrder, err := b.queryOrder(order)
	if err != nil {
		return "", getClauseBuilderError("order", "order array", err)
	}

	qLimitOffset := LimitOffset(limit, offset)
	qWhere, err := b.queryFilters(filters)
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

func (b *QueryBuilder) SelectCount(filters *filters.Filters) (string, error) {
	query := b.queryContainer.SelectCountPrefix

	qWhere, err := b.queryFilters(filters)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}

func (b *QueryBuilder) Delete(filters *filters.Filters) (string, error) {
	query := b.queryContainer.DeletePrefix

	qWhere, err := b.queryFilters(filters)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}

func (b *QueryBuilder) DeleteReturningID(filters *filters.Filters) (string, error) {
	idFieldColumnName, ok := b.queryContainer.FieldColumnName["ID"]
	if !ok {
		panic(idFieldNotFoundInQueryContainerError)
	}

	query := b.queryContainer.DeletePrefix

	qWhere, err := b.queryFilters(filters)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	query += fmt.Sprintf(` RETURNING "%s"`, idFieldColumnName)

	return query, nil
}

func (b *QueryBuilder) Update(values map[string]interface{}, filters *filters.Filters) (string, error) {
	query := b.queryContainer.UpdatePrefix

	qSet, err := b.querySet(values)
	if err != nil {
		return "", getClauseBuilderError("set", "values map", err)
	}

	query += " " + qSet

	qWhere, err := b.queryFilters(filters)
	if err != nil {
		return "", getClauseBuilderError("where", "filters", err)
	}

	if qWhere != "" {
		query += " WHERE " + qWhere
	}

	return query, nil
}
