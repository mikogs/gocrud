// Package dialect has what differs between the databases that gocrud supports, PostgreSQL and SQLite, in the SQL of
// the queries. Everything else is built in the same way for both of them.
package dialect

import (
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Dialect is the SQL of a database.
type Dialect interface {
	// Placeholder returns the placeholder of the n-th value of a query, from 1, e.g. $1 or ?.
	Placeholder(n int) string
	// IDColumn returns the definition of the column of the ID: a UUID for a string ID, and otherwise an integer that
	// the database assigns.
	IDColumn(uuid bool) string
	// ColumnType returns the type of the column of a field of a kind of Go type, e.g. "int64", or of "time.Time", or of
	// "json" for a field stored as JSON, and its default value.
	ColumnType(fieldType string) (columnType string, defaultValue string)
	// Literal returns a value that a field is written to the database as, e.g. the value of a Valuer, as an SQL
	// literal, for the default value of a column.
	Literal(value driver.Value) string
	// UUIDType returns the type of the columns with UUIDs, which have no default value, or an empty string when UUIDs
	// are stored as strings, in the columns of strings.
	UUIDType() string
	// AddColumn returns the statements that add a column to an existing table, with the definition of the column with
	// or without its UNIQUE constraint.
	AddColumn(table string, column string, definition func(unique bool) string, unique bool) []string
	// TextColumn returns a column as text, for LIKE and regular expressions. notText is set for a column whose values
	// are not text, e.g. numbers.
	TextColumn(column string, notText bool) string
	// MatchOperator returns the operator of regular expressions, or an empty string when the database has none.
	MatchOperator() string

	// TimeFormat returns the format of the times, which the database stores as text in UTC, or an empty string when the
	// database has a type of times.
	TimeFormat() string
	// TruncateTime returns an expression of a time column truncated to a unit, day, month or year, as a time in UTC,
	// and false for another unit.
	TruncateTime(column string, unit string) (string, bool)

	// The queries that read the tables of the database, for migrations. Their only value is the name of the table.

	// ColumnsQuery returns the names, the types, the lengths and whether they are nullable, of the columns of a table.
	ColumnsQuery() string
	// UniqueColumnsQuery returns the names of the columns that are unique on their own: with a unique index on the
	// column only, which is also how UNIQUE constraints are implemented. Partial indexes are not included.
	UniqueColumnsQuery() string
	// ForeignKeysQuery returns the column, the referenced table and the action on delete of the foreign keys of single
	// columns of a table.
	ForeignKeysQuery() string
	// IndexesQuery returns the names of the indexes of a table.
	IndexesQuery() string
	// ForeignKeysEnforcedQuery returns whether the database enforces foreign keys, or it is an empty string when the
	// database always does.
	ForeignKeysEnforcedQuery() string
	// StoredType returns the type of a column, e.g. "VARCHAR(20)", as ColumnsQuery returns it, and its length, or an
	// empty type when the type is not known, and is not compared.
	StoredType(columnType string) (string, int)
	// StoredUUIDType returns the type of the columns with UUIDs, as ColumnsQuery returns it.
	StoredUUIDType() string
	// StoredIndexName returns the name of an index as the database stores it.
	StoredIndexName(name string) string
	// UniqueKey returns how the errors of the database name a violation of a unique index of a name, or of the UNIQUE
	// constraint of a column when name is empty: PostgreSQL by the name of the constraint or the index, e.g.
	// "user_email_key", and SQLite by the columns, e.g. "user.email".
	UniqueKey(table string, name string, columns []string) string
}

const (
	NamePostgres = "postgres"
	NameSQLite   = "sqlite"
)

// For returns the dialect of a name: SQLite for NameSQLite, and PostgreSQL otherwise.
func For(name string) Dialect {
	if name == NameSQLite {
		return SQLite{}
	}
	return Postgres{}
}

// literal returns value as an SQL literal, with the literals of bools, bytes and times of a dialect.
func literal(value driver.Value, boolLiteral func(bool) string, bytesLiteral func(string) string, timeLiteral func(time.Time) string) string {
	switch v := value.(type) {
	case nil:
		return "NULL"
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case bool:
		return boolLiteral(v)
	case []byte:
		return bytesLiteral(hex.EncodeToString(v))
	case time.Time:
		return timeLiteral(v)
	case string:
		return "'" + strings.ReplaceAll(v, "'", "''") + "'"
	default:
		return literal(fmt.Sprint(v), boolLiteral, bytesLiteral, timeLiteral)
	}
}
