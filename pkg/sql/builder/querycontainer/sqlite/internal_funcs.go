package sqlite

import (
	"fmt"
	"time"
)

// columnDefinitionFromField maps a struct field type to the definition of its database column.
// Nullable columns do not have NOT NULL and DEFAULT, so that their values can be NULL.
func columnDefinitionFromField(fieldName string, fieldType string, isUnique bool, overrideColumnType string, isNullable bool) string {
	if fieldName == "ID" {
		return "INTEGER PRIMARY KEY"
	}

	columnType, defaultValue := columnTypeFromField(fieldType)
	if overrideColumnType != "" {
		columnType = overrideColumnType
		defaultValue = "''"
	}

	definition := columnType
	if !isNullable {
		definition += " NOT NULL DEFAULT " + defaultValue
	}

	if isUnique {
		definition += " UNIQUE"
	}

	return definition
}

// columnTypeFromField returns the column type and the default value for a field type.
func columnTypeFromField(fieldType string) (string, string) {
	switch fieldType {
	case "string":
		return "TEXT", "''"
	case "bool":
		return "INTEGER", "0"
	case "float32", "float64":
		return "REAL", "0"
	case "time.Time":
		// the driver reads DATETIME columns as time.Time, the default is the zero value of time.Time
		return "DATETIME", "'" + TimeValue(time.Time{}) + "'"
	default:
		// all int/uint variants
		return "INTEGER", "0"
	}
}

// columnType returns the type of the column for a field type, or the type set in the field tag.
func columnType(fieldType string, overrideColumnType string) string {
	if overrideColumnType != "" {
		return overrideColumnType
	}

	columnType, _ := columnTypeFromField(fieldType)
	return columnType
}

// addColumnStatements returns the statements that add a column to an existing table. SQLite cannot add a column
// with a UNIQUE constraint, so a unique index is created for it instead.
func addColumnStatements(rawTableName string, columnName string, fieldName string, fieldType string, isUnique bool, overrideColumnType string, isNullable bool) []string {
	definition := columnDefinitionFromField(fieldName, fieldType, false, overrideColumnType, isNullable)
	statements := []string{
		fmt.Sprintf(`ALTER TABLE "%s" ADD COLUMN %s %s`, rawTableName, QuoteColumn(columnName), definition),
	}

	if isUnique {
		statements = append(statements, fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS "uniq_%s_%s" ON "%s" (%s)`, rawTableName, columnName, rawTableName, QuoteColumn(columnName)))
	}

	return statements
}
