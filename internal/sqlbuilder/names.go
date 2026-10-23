package sqlbuilder

import (
	"fmt"
	"strings"
	"unicode"
)

// fieldToColumn converts the name of a struct field to the name of its column, e.g. customer_id for CustomerID.
func fieldToColumn(s string) string {
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

// quoteName quotes the name of a column, and only the name of a column with an alias, e.g. t1."name" for t1.name.
func quoteName(column string) string {
	if strings.Contains(column, ".") {
		parts := strings.SplitN(column, ".", 2)
		if len(parts) == 2 {
			return fmt.Sprintf(`%s."%s"`, parts[0], parts[1])
		}
	}

	return fmt.Sprintf(`"%s"`, column)
}

// columnWithoutAlias returns the name of a column without its alias, e.g. "name" for "t1.name".
func columnWithoutAlias(column string) string {
	if i := strings.Index(column, "."); i >= 0 {
		return column[i+1:]
	}

	return column
}

// quoteColumn quotes the name of a column, keeping its alias, e.g. t1."name", only when withAlias is true.
func quoteColumn(columnName string, withAlias bool) string {
	if !withAlias {
		columnName = columnWithoutAlias(columnName)
	}

	return quoteName(columnName)
}
