package sqlite

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	pkgfilters "github.com/mikogs/gocrud/pkg/filters"
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

func (b *QueryBuilder) queryFilters(filters *pkgfilters.Filters, withAlias bool) (string, error) {
	if filters == nil || len(*filters) == 0 {
		return "", nil
	}

	sortedNames := make([]string, 0, len(*filters))
	for name := range *filters {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	parts := make([]string, 0, len(sortedNames))

	for _, name := range sortedNames {
		if name == pkgfilters.Raw {
			continue
		}

		fieldInfo, ok := b.structInfo.Fields[name]
		if !ok {
			return "", getFieldInfosIsNilError()
		}
		if fieldInfo.Ignored {
			return "", getFieldIsIgnoredError()
		}

		columnName, ok := b.queryContainer.FieldColumnName[name]
		if !ok {
			return "", getColumnNameBuilderError("filter")
		}

		fieldColumn := quoteColumn(columnName, withAlias)

		var clause string
		switch (*filters)[name].Op {
		case pkgfilters.OpLike:
			clause = fmt.Sprintf(`%s LIKE ?`, fieldColumn)
		case pkgfilters.OpMatch:
			return "", fmt.Errorf("OpMatch (regex) is not supported for SQLite")
		case pkgfilters.OpNotEqual:
			clause = fmt.Sprintf(`%s!=?`, fieldColumn)
		case pkgfilters.OpGreater:
			clause = fmt.Sprintf(`%s>?`, fieldColumn)
		case pkgfilters.OpLower:
			clause = fmt.Sprintf(`%s<?`, fieldColumn)
		case pkgfilters.OpGreaterOrEqual:
			clause = fmt.Sprintf(`%s>=?`, fieldColumn)
		case pkgfilters.OpLowerOrEqual:
			clause = fmt.Sprintf(`%s<=?`, fieldColumn)
		case pkgfilters.OpBit:
			clause = fmt.Sprintf(`%s&?>0`, fieldColumn)
		default:
			clause = fmt.Sprintf(`%s=?`, fieldColumn)
		}

		parts = append(parts, clause)
	}

	queryWhere := strings.Join(parts, " AND ")

	// _raw
	rawQueryArr, ok := (*filters)[pkgfilters.Raw]
	if !ok {
		return queryWhere, nil
	}

	rawQuery, rawValues, err := rawQueryAndValues(rawQueryArr)
	if err != nil {
		return "", err
	}

	if rawQuery == "" {
		return queryWhere, nil
	}

	if queryWhere != "" {
		queryWhere = fmt.Sprintf("(%s)", queryWhere)
		if rawQueryArr.Op == pkgfilters.OpOR {
			queryWhere += " OR "
		} else {
			queryWhere += " AND "
		}
	}

	queryWhere += "("

	// Replace .FieldName with the quoted column name
	fieldsInRaw := regexpFieldInRaw.FindAllString(rawQuery, -1)
	alreadyReplaced := map[string]bool{}
	for _, fieldInRaw := range fieldsInRaw {
		if alreadyReplaced[fieldInRaw] {
			continue
		}
		fieldName := strings.TrimPrefix(fieldInRaw, ".")
		columnName, ok := b.queryContainer.FieldColumnName[fieldName]
		if !ok {
			return "", getColumnNameBuilderError("raw query")
		}
		rawQuery = strings.ReplaceAll(rawQuery, fieldInRaw, quoteColumn(columnName, withAlias))
		alreadyReplaced[fieldInRaw] = true
	}

	// Expand slice values into ?,?,? by splitting on ? and reassembling.
	// Each value in rawQueryArr.Val[1:] corresponds to exactly one ? in the
	// raw query; slices expand that single ? into ?,?,? while scalar values
	// keep it as ?.
	numRaw := len(rawValues)
	if numRaw > 1 {
		qParts := strings.Split(rawQuery, "?")
		var rebuilt strings.Builder
		rebuilt.WriteString(qParts[0])
		for j := 1; j < numRaw; j++ {
			elem := rawValues[j]
			elemVal := reflect.ValueOf(elem)
			if elemVal.Kind() == reflect.Slice || elemVal.Kind() == reflect.Array {
				n := elemVal.Len()
				if n == 0 {
					// no placeholder — consume the ? without emitting anything
				} else {
					rebuilt.WriteString(strings.Repeat("?,", n)[:n*2-1])
				}
			} else {
				rebuilt.WriteString("?")
			}
			if j < len(qParts) {
				rebuilt.WriteString(qParts[j])
			}
		}
		rawQuery = rebuilt.String()
	}

	queryWhere += rawQuery + ")"

	return queryWhere, nil
}

// quoteColumn quotes the column name, keeping its alias (eg. "t1.name") only when withAlias is true.
func quoteColumn(columnName string, withAlias bool) string {
	if !withAlias {
		columnName = sqliteQueryContainer.ColumnWithoutAlias(columnName)
	}

	return sqliteQueryContainer.QuoteColumn(columnName)
}

// rawQueryAndValues returns the query and the values of the raw filter, which must be
// an []interface{} starting with the query string.
func rawQueryAndValues(rawFilter pkgfilters.OpVal) (string, []interface{}, error) {
	rawValues, ok := rawFilter.Val.([]interface{})
	if !ok {
		return "", nil, getRawFilterBuilderError()
	}

	if len(rawValues) == 0 {
		return "", nil, nil
	}

	rawQuery, ok := rawValues[0].(string)
	if !ok {
		return "", nil, getRawFilterBuilderError()
	}

	return rawQuery, rawValues, nil
}
