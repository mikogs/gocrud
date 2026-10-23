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

		// a field of a related struct, e.g. Customer.Name, is selected by a subquery, which has no values, and is NULL when
		// there is no related row, which comes last in both directions in all the databases
		if strings.Contains(field, ".") {
			column, err := where.OrderColumn(field, b.WhereDialect("", withAlias, func() string { return "" }))
			if err != nil {
				return "", err
			}
			parts = append(parts, fmt.Sprintf(`%s %s NULLS LAST`, column, queryDirection))
			continue
		}

		columnName, ok := b.queryContainer.FieldColumnName[field]
		if !ok {
			return "", getColumnNameBuilderError("order")
		}
		if b.structInfo.Fields[field].Password {
			return "", fmt.Errorf("%w: %s", where.ErrPasswordField, field)
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
	return where.Build(f, b.WhereDialect("", withAlias, func() string { return "?" }))
}

// WhereDialect returns the dialect of the conditions on the fields of the struct. The columns are qualified with
// qualifier, e.g. the alias of the table in a subquery, unless it is empty.
func (b *QueryBuilder) WhereDialect(qualifier string, withAlias bool, placeholder func() string) where.Dialect {
	d := where.Dialect{
		Column: func(field string) (string, error) {
			column, err := b.filterColumn(field, withAlias)
			if err != nil || qualifier == "" {
				return column, err
			}
			return qualifier + "." + column, nil
		},
		TextColumn: func(_ string, column string) string {
			return column
		},
		Placeholder: placeholder,
	}

	if b.Relation != nil {
		outer := qualifier
		if outer == "" {
			outer = b.queryContainer.TableName
		}
		d.Relation = func(field string, alias string) (where.Related, error) {
			if withAlias {
				return where.Related{}, fmt.Errorf("%w: %s: the struct has its own select query", where.ErrRelationPath, field)
			}
			return b.Relation(outer, field, alias, placeholder)
		}
	}

	if b.Subquery != nil {
		d.Subquery = func(obj interface{}, field string, alias string) (where.Selected, error) {
			return b.Subquery(obj, field, alias, placeholder)
		}
	}

	return d
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
	if fieldInfo.Password {
		return "", fmt.Errorf("%w: %s", where.ErrPasswordField, field)
	}

	columnName, ok := b.queryContainer.FieldColumnName[field]
	if !ok {
		return "", getColumnNameBuilderError("filter")
	}

	return quoteColumn(columnName, withAlias), nil
}
