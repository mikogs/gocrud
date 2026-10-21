package postgres

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	pkgfilters "github.com/mikogs/gocrud/pkg/filters"
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
		if direction == strings.ToLower("desc") {
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

func (b *QueryBuilder) queryFilters(filters *pkgfilters.Filters, firstValueNum int, withAlias bool) (string, error) {
	if filters == nil || len(*filters) == 0 {
		return "", nil
	}

	sortedNames := make([]string, 0, len(*filters))
	for name := range *filters {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	queryWhere := ""
	valueNum := firstValueNum

	for _, name := range sortedNames {
		// _raw is a special entry that allows an almost-raw SQL query
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

		if (*filters)[name].IsNull() {
			queryWhere += " AND " + nullCondition(quoteColumn(columnName, withAlias), (*filters)[name].Op)
			continue
		}

		var fieldColumn string
		if b.queryContainer.ColumnNotString[columnName] && ((*filters)[name].Op == pkgfilters.OpLike || (*filters)[name].Op == pkgfilters.OpMatch) {
			fieldColumn = fmt.Sprintf(`CAST(%s AS TEXT)`, quoteColumn(columnName, withAlias))
		} else {
			fieldColumn = quoteColumn(columnName, withAlias)
		}

		if (*filters)[name].Op == pkgfilters.OpIn {
			values, ok := (*filters)[name].InValues()
			if !ok {
				return "", getInFilterBuilderError()
			}
			queryWhere += " AND " + inCondition(fieldColumn, len(values), valueNum)
			valueNum += len(values)
			continue
		}

		switch (*filters)[name].Op {
		case pkgfilters.OpLike:
			queryWhere += fmt.Sprintf(` AND %s LIKE $%d`, fieldColumn, valueNum)
		case pkgfilters.OpMatch:
			queryWhere += fmt.Sprintf(` AND %s ~ $%d`, fieldColumn, valueNum)
		case pkgfilters.OpNotEqual:
			queryWhere += fmt.Sprintf(` AND %s!=$%d`, fieldColumn, valueNum)
		case pkgfilters.OpGreater:
			queryWhere += fmt.Sprintf(` AND %s>$%d`, fieldColumn, valueNum)
		case pkgfilters.OpLower:
			queryWhere += fmt.Sprintf(` AND %s<$%d`, fieldColumn, valueNum)
		case pkgfilters.OpGreaterOrEqual:
			queryWhere += fmt.Sprintf(` AND %s>=$%d`, fieldColumn, valueNum)
		case pkgfilters.OpLowerOrEqual:
			queryWhere += fmt.Sprintf(` AND %s<=$%d`, fieldColumn, valueNum)
		case pkgfilters.OpBit:
			queryWhere += fmt.Sprintf(` AND %s&$%d>0`, fieldColumn, valueNum)
		default:
			queryWhere += fmt.Sprintf(` AND %s=$%d`, fieldColumn, valueNum)
		}

		valueNum++
	}

	if queryWhere != "" {
		queryWhere = queryWhere[5:]
	}

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

		conjunction := rawQueryArr.Op
		if conjunction != pkgfilters.OpOR {
			queryWhere += " AND "
		} else {
			queryWhere += " OR "
		}
	}

	queryWhere += "("

	fieldsInRaw := regexpFieldInRaw.FindAllString(rawQuery, -1)
	alreadyReplaced := map[string]bool{}
	for _, fieldInRaw := range fieldsInRaw {
		if alreadyReplaced[fieldInRaw] {
			continue
		}

		fieldName := strings.Replace(fieldInRaw, ".", "", 1)

		columnName, ok := b.queryContainer.FieldColumnName[fieldName]
		if !ok {
			return "", getColumnNameBuilderError("raw query")
		}

		rawQuery = strings.ReplaceAll(rawQuery, fieldInRaw, quoteColumn(columnName, withAlias))
		alreadyReplaced[fieldInRaw] = true
	}

	for j := 1; j < len(rawValues); j++ {
		rawValue := reflect.ValueOf(rawValues[j])
		if rawValue.Kind() != reflect.Slice && rawValue.Kind() != reflect.Array {
			// Value is a single value so just replace ? with $x, eg $2
			rawQuery = strings.Replace(rawQuery, "?", fmt.Sprintf("$%d", valueNum), 1)
			valueNum++
			continue
		}

		rawNumValue := rawValue.Len()

		queryVal := ""
		for k := 0; k < rawNumValue; k++ {
			if k == 0 {
				queryVal += fmt.Sprintf("$%d", valueNum)
				valueNum++
				continue
			}
			queryVal += fmt.Sprintf(",$%d", valueNum)
			valueNum++
		}
		rawQuery = strings.Replace(rawQuery, "?", queryVal, 1)
	}

	queryWhere += rawQuery + ")"

	return queryWhere, nil
}

// quoteColumn quotes the column name, keeping its alias (eg. "t1.name") only when withAlias is true.
func quoteColumn(columnName string, withAlias bool) string {
	if !withAlias {
		columnName = postgresQueryContainer.ColumnWithoutAlias(columnName)
	}

	return postgresQueryContainer.QuoteColumn(columnName)
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

// nullCondition returns an IS NULL condition for a filter with OpEqual, and IS NOT NULL for OpNotEqual.
func nullCondition(column string, op int) string {
	if op == pkgfilters.OpNotEqual {
		return column + " IS NOT NULL"
	}

	return column + " IS NULL"
}

// inCondition returns an IN condition with num placeholders, starting with $first. An empty list matches nothing.
func inCondition(column string, num int, first int) string {
	if num == 0 {
		return "1=0"
	}

	placeholders := make([]string, num)
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", first+i)
	}

	return column + " IN (" + strings.Join(placeholders, ",") + ")"
}
