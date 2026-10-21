package postgres

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/mikogs/gocrud/pkg/structinfo"
)

// FieldToColumn converts struct field name to database column name.
func FieldToColumn(s string) string {
	if s == "ID" {
		return "id"
	}

	o := ""

	var prev rune
	for i, ch := range s {
		if i == 0 {
			o += strings.ToLower(string(ch))
			prev = ch
			continue
		}

		if unicode.IsUpper(ch) {
			if prev == 'I' && ch == 'D' {
				o += strings.ToLower(string(ch))
				continue
			}

			o += "_" + strings.ToLower(string(ch))
			prev = ch
			continue
		}

		o += string(ch)
		prev = ch
	}

	return o
}

// QuoteColumn quotes a column name.
func QuoteColumn(column string) string {
	// if column is in format of alias.name then quote only the name.
	if strings.Contains(column, ".") {
		parts := strings.SplitN(column, ".", 2)
		if len(parts) == 2 {
			return fmt.Sprintf(`%s."%s"`, parts[0], parts[1])
		}
	}

	return fmt.Sprintf(`"%s"`, column)
}

// ColumnWithoutAlias returns the column name without its alias, eg. "name" for "t1.name".
func ColumnWithoutAlias(column string) string {
	if i := strings.Index(column, "."); i >= 0 {
		return column[i+1:]
	}

	return column
}

// indexStatements builds a CREATE INDEX statement for each index in 'indexes'.
// Indexes referencing only ignored fields (absent from fieldColumnName) are skipped.
func indexStatements(rawTableName string, quotedTableName string, indexes []*structinfo.IndexInfo, fieldColumnName map[string]string) []string {
	statements := make([]string, 0, len(indexes))

	for _, idx := range indexes {
		columns := make([]string, 0, len(idx.Fields))
		columnNames := make([]string, 0, len(idx.Fields))

		for _, fieldName := range idx.Fields {
			columnName, ok := fieldColumnName[fieldName]
			if !ok {
				continue
			}
			columnName = ColumnWithoutAlias(columnName)
			columns = append(columns, QuoteColumn(columnName))
			columnNames = append(columnNames, columnName)
		}

		if len(columns) == 0 {
			continue
		}

		indexName := idx.Name
		if indexName == "" {
			indexName = strings.Join(columnNames, "_")
		}

		unique := ""
		if idx.Unique {
			unique = "UNIQUE "
		}

		statements = append(statements, fmt.Sprintf(`CREATE %sINDEX IF NOT EXISTS "idx_%s_%s" ON %s (%s)`, unique, rawTableName, indexName, quotedTableName, strings.Join(columns, ",")))
	}

	return statements
}

// placeholders returns num comma-separated placeholders, starting with $first.
func placeholders(first int, num int) string {
	values := make([]string, num)
	for i := range values {
		values[i] = fmt.Sprintf("$%d", first+i)
	}

	return strings.Join(values, ",")
}

// setClause returns a SET clause for the quoted columns, with placeholders starting with $first.
func setClause(columns []string, first int) string {
	values := make([]string, len(columns))
	for i, column := range columns {
		values[i] = fmt.Sprintf("%s=$%d", column, first+i)
	}

	return strings.Join(values, ",")
}
