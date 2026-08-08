package sqlite

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/mikolajgasior/gocrud/pkg/structinfo"
)

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

func QuoteColumn(column string) string {
	if strings.Contains(column, ".") {
		parts := strings.SplitN(column, ".", 2)
		if len(parts) == 2 {
			return fmt.Sprintf(`%s."%s"`, parts[0], parts[1])
		}
	}
	return fmt.Sprintf(`"%s"`, column)
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
