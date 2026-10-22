package sqlite

import (
	"fmt"
	"sort"
	"strings"

	pkgfilters "github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/sql/builder/querybuilder/where"
	sqliteQueryContainer "github.com/mikogs/gocrud/pkg/sql/builder/querycontainer/sqlite"
)

func (b *QueryBuilder) queryOrder(order []string, withAlias bool) (string, error) {
	if len(order) == 0 {
		return "", nil
	}
	if len(order)%2 != 0 {
		return "", getOrderBuilderError()
	}

	parts := make([]string, 0, len(order)/2)
	for i := 0; i < len(order); i += 2 {
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

		parts = append(parts, fmt.Sprintf(`%s %s`, quoteColumn(columnName, withAlias), queryDirection))
	}

	return strings.Join(parts, ","), nil
}

func (b *QueryBuilder) querySet(values map[string]interface{}) (string, error) {
	if len(values) == 0 {
		return "", nil
	}

	columns := make([]string, 0, len(values))
	for value := range values {
		fieldInfo, ok := b.structInfo.Fields[value]
		if !ok {
			return "", getFieldInfosIsNilError()
		}
		if fieldInfo.Ignored {
			return "", getFieldIsIgnoredError()
		}

		columnName, ok := b.queryContainer.FieldColumnName[value]
		if !ok {
			return "", getColumnNameBuilderError("value")
		}

		columns = append(columns, columnName)
	}

	sort.Strings(columns)

	parts := make([]string, len(columns))
	for i, col := range columns {
		parts[i] = fmt.Sprintf(`%s=?`, sqliteQueryContainer.QuoteColumn(sqliteQueryContainer.ColumnWithoutAlias(col)))
	}

	return strings.Join(parts, ","), nil
}

func (b *QueryBuilder) queryFilters(f pkgfilters.Filter, withAlias bool) (string, error) {
	return where.Build(f, where.Dialect{
		Column: func(field string) (string, error) {
			return b.filterColumn(field, withAlias)
		},
		TextColumn: func(_ string, column string) string {
			return column
		},
		Placeholder: func() string {
			return "?"
		},
	})
}

// quoteColumn quotes the column name, keeping its alias (eg. "t1.name") only when withAlias is true.
func quoteColumn(columnName string, withAlias bool) string {
	if !withAlias {
		columnName = sqliteQueryContainer.ColumnWithoutAlias(columnName)
	}

	return sqliteQueryContainer.QuoteColumn(columnName)
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
