package dialect

import (
	"fmt"
	"time"
)

type SQLite struct{}

// SQLiteTimeFormat is the format that SQLite stores times in. All the times are stored in UTC and have the same
// length, so that comparing them as strings gives the same result as comparing the times.
const SQLiteTimeFormat = "2006-01-02 15:04:05.000000000-07:00"

// SQLiteTimeValue returns t as a string in SQLiteTimeFormat.
func SQLiteTimeValue(t time.Time) string {
	return t.UTC().Format(SQLiteTimeFormat)
}

func (SQLite) Placeholder(int) string { return "?" }

func (SQLite) IDColumn(uuid bool) string {
	if uuid {
		return "TEXT PRIMARY KEY"
	}
	return "INTEGER PRIMARY KEY"
}

func (SQLite) ColumnType(fieldType string) (string, string) {
	switch fieldType {
	case "string":
		return "TEXT", "''"
	case "bool":
		return "INTEGER", "0"
	case "float32", "float64":
		return "REAL", "0"
	case "time.Time":
		// the driver reads DATETIME columns as time.Time, the default is the zero value of time.Time
		return "DATETIME", "'" + SQLiteTimeValue(time.Time{}) + "'"
	default:
		// all the integers
		return "INTEGER", "0"
	}
}

func (SQLite) UUIDType() string { return "" }

// AddColumn adds a column without its UNIQUE constraint, which SQLite cannot add, and a unique index instead.
func (SQLite) AddColumn(table string, column string, definition func(unique bool) string, unique bool) []string {
	statements := []string{fmt.Sprintf(`ALTER TABLE "%s" ADD COLUMN "%s" %s`, table, column, definition(false))}
	if unique {
		statements = append(statements, fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS "uniq_%s_%s" ON "%s" ("%s")`, table, column, table, column))
	}
	return statements
}

func (SQLite) TextColumn(column string, _ bool) string { return column }

func (SQLite) MatchOperator() string { return "" }

func (SQLite) TimeFormat() string { return SQLiteTimeFormat }

// TruncateTime replaces the end of the time, which SQLite stores as text in SQLiteTimeFormat, with a fixed length.
func (SQLite) TruncateTime(column string, unit string) (string, bool) {
	switch unit {
	case "day":
		return "substr(" + column + ",1,10)||' 00:00:00.000000000+00:00'", true
	case "month":
		return "substr(" + column + ",1,7)||'-01 00:00:00.000000000+00:00'", true
	case "year":
		return "substr(" + column + ",1,4)||'-01-01 00:00:00.000000000+00:00'", true
	}
	return "", false
}

func (SQLite) ColumnsQuery() string {
	return `SELECT name, type, 0, "notnull" = 0 AND pk = 0 FROM pragma_table_info(?)`
}

func (SQLite) UniqueColumnsQuery() string {
	return `SELECT ii.name FROM pragma_index_list(?) AS il, pragma_index_info(il.name) AS ii ` +
		`WHERE il."unique" = 1 AND il.partial = 0 AND (SELECT COUNT(*) FROM pragma_index_info(il.name)) = 1`
}

func (SQLite) ForeignKeysQuery() string {
	return `SELECT "from", "table", on_delete FROM pragma_foreign_key_list(?) GROUP BY id HAVING COUNT(*) = 1`
}

func (SQLite) IndexesQuery() string { return `SELECT name FROM pragma_index_list(?)` }

// ForeignKeysEnforcedQuery returns the foreign_keys pragma, as SQLite enforces foreign keys only when it is on.
func (SQLite) ForeignKeysEnforcedQuery() string { return "PRAGMA foreign_keys" }

// StoredType returns the type as it is, as SQLite stores the types of the CREATE TABLE statements.
func (SQLite) StoredType(columnType string) (string, int) { return columnType, 0 }

func (SQLite) StoredUUIDType() string { return "TEXT" }

func (SQLite) StoredIndexName(name string) string { return name }
