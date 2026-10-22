package postgres

import (
	"fmt"
	"sort"
	"strings"

	pkgfilters "github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/sql/builder/querybuilder/where"
	postgresQueryContainer "github.com/mikogs/gocrud/pkg/sql/builder/querycontainer/postgres"
)

func (b *QueryBuilder) queryOrder(order []string, withAlias bool) (string, error) {
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

		columnName, ok := b.queryContainer.FieldColumnName[field]
		if !ok {
			return "", getColumnNameBuilderError("order")
		}

		qOrder += fmt.Sprintf(`,%s %s`, quoteColumn(columnName, withAlias), queryDirection)
	}

	if qOrder == "" {
		return qOrder, nil
	}

	return qOrder[1:], nil
}

func (b *QueryBuilder) querySet(values map[string]interface{}) (string, int, error) {
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

		columnName, ok := b.queryContainer.FieldColumnName[value]
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
		querySet += fmt.Sprintf(`,%s=$%d`, postgresQueryContainer.QuoteColumn(postgresQueryContainer.ColumnWithoutAlias(columns[i-1])), i)
	}

	return querySet[1:], numColumn, nil
}

func (b *QueryBuilder) queryFilters(f pkgfilters.Filter, firstValueNum int, withAlias bool) (string, error) {
	valueNum := firstValueNum

	return where.Build(f, where.Dialect{
		Column: func(field string) (string, error) {
			return b.filterColumn(field, withAlias)
		},
		TextColumn: func(field string, column string) string {
			if b.queryContainer.ColumnNotString[b.queryContainer.FieldColumnName[field]] {
				return "CAST(" + column + " AS TEXT)"
			}
			return column
		},
		Placeholder: func() string {
			valueNum++
			return fmt.Sprintf("$%d", valueNum-1)
		},
		MatchOperator: "~",
	})
}

// quoteColumn quotes the column name, keeping its alias (eg. "t1.name") only when withAlias is true.
func quoteColumn(columnName string, withAlias bool) string {
	if !withAlias {
		columnName = postgresQueryContainer.ColumnWithoutAlias(columnName)
	}

	return postgresQueryContainer.QuoteColumn(columnName)
}

// filterColumn returns the quoted column of a field that is used in a filter.
func (b *QueryBuilder) filterColumn(field string, withAlias bool) (string, error) {
	fieldInfo, ok := b.structInfo.Fields[field]
	if !ok {
		return "", getFieldInfosIsNilError()
	}
	if fieldInfo.Ignored {
		return "", getFieldIsIgnoredError()
	}

	columnName, ok := b.queryContainer.FieldColumnName[field]
	if !ok {
		return "", getColumnNameBuilderError("filter")
	}

	return quoteColumn(columnName, withAlias), nil
}
