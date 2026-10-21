package postgres

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	pkgfilters "github.com/mikogs/gocrud/pkg/filters"
	postgresQueryContainer "github.com/mikogs/gocrud/pkg/sql/builder/querycontainer/postgres"
)

func (b *QueryBuilder) queryOrder(order []string) (string, error) {
	if len(order) == 0 {
		return "", nil
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

		qOrder += fmt.Sprintf(`,%s %s`, postgresQueryContainer.QuoteColumn(columnName), queryDirection)
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
		querySet += fmt.Sprintf(`,%s=$%d`, postgresQueryContainer.QuoteColumn(columns[i-1]), i)
	}

	return querySet[1:], numColumn, nil
}

func (b *QueryBuilder) queryFilters(filters *pkgfilters.Filters, firstValueNum int) (string, error) {
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

		var fieldColumn string
		if b.queryContainer.ColumnNotString[columnName] && ((*filters)[name].Op == pkgfilters.OpLike || (*filters)[name].Op == pkgfilters.OpMatch) {
			fieldColumn = fmt.Sprintf(`CAST(%s AS TEXT)`, postgresQueryContainer.QuoteColumn(columnName))
		} else {
			fieldColumn = fmt.Sprintf(`%s`, postgresQueryContainer.QuoteColumn(columnName))
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
	if !ok || len(rawQueryArr.Val.([]interface{})) == 0 {
		return queryWhere, nil
	}

	rawQuery := rawQueryArr.Val.([]interface{})[0].(string)
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

		rawQuery = strings.ReplaceAll(rawQuery, fieldInRaw, fmt.Sprintf(`%s`, postgresQueryContainer.QuoteColumn(columnName)))
		alreadyReplaced[fieldInRaw] = true
	}

	numRaw := len((*filters)[pkgfilters.Raw].Val.([]interface{}))
	for j := 1; j < numRaw; j++ {
		rawType := reflect.TypeOf((*filters)[pkgfilters.Raw].Val.([]interface{})[j])
		if rawType.Kind() != reflect.Slice && rawType.Kind() != reflect.Array {
			// Value is a single value so just replace ? with $x, eg $2
			rawQuery = strings.Replace(rawQuery, "?", fmt.Sprintf("$%d", valueNum), 1)
			valueNum++
			continue
		}

		rawNumValue := reflect.ValueOf((*filters)[pkgfilters.Raw].Val.([]interface{})[j]).Len()

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
