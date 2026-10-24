package dialect

import (
	"database/sql/driver"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Postgres struct{}

func (Postgres) Placeholder(n int) string { return fmt.Sprintf("$%d", n) }

// MaxPlaceholders returns the limit of the protocol of PostgreSQL, which sends the number of values in 16 bits.
func (Postgres) MaxPlaceholders() int { return 65535 }

func (Postgres) IDColumn(uuid bool) string {
	if uuid {
		return "UUID PRIMARY KEY"
	}
	return "SERIAL PRIMARY KEY"
}

func (Postgres) ColumnType(fieldType string) (string, string) {
	switch fieldType {
	case "bool":
		return "BOOLEAN", "false"
	// the integer types of PostgreSQL are signed, so an unsigned type gets the next larger one, except uint64, whose
	// values from 2^63 cannot be stored
	case "int64", "int", "uint64", "uint", "uint32":
		return "BIGINT", "0"
	case "int32", "uint16":
		return "INTEGER", "0"
	case "int16", "int8", "uint8":
		return "SMALLINT", "0"
	case "float64":
		return "DOUBLE PRECISION", "0"
	case "float32":
		return "REAL", "0"
	case "time.Time":
		// the default is the zero value of time.Time
		return "TIMESTAMPTZ", "'0001-01-01 00:00:00+00'"
	case "[]byte":
		return "BYTEA", "''"
	case "json":
		return "JSONB", "'null'"
	default:
		return "VARCHAR(255)", "''"
	}
}

func (Postgres) Literal(value driver.Value) string {
	return literal(value,
		func(b bool) string { return strconv.FormatBool(b) },
		func(hex string) string { return `'\x` + hex + "'" },
		func(t time.Time) string { return "'" + t.UTC().Format("2006-01-02 15:04:05.999999-07") + "'" })
}

func (Postgres) UUIDType() string { return "UUID" }

func (Postgres) AddColumn(table string, column string, definition func(unique bool) string, unique bool) []string {
	return []string{fmt.Sprintf(`ALTER TABLE "%s" ADD COLUMN "%s" %s`, table, column, definition(unique))}
}

func (Postgres) TextColumn(column string, notText bool) string {
	if notText {
		return "CAST(" + column + " AS TEXT)"
	}
	return column
}

func (Postgres) MatchOperator() string { return "~" }

func (Postgres) TimeFormat() string { return "" }

func (Postgres) TruncateTime(column string, unit string) (string, bool) {
	switch unit {
	case "day", "month", "year":
		return "date_trunc('" + unit + "'," + column + " AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'", true
	}
	return "", false
}

func (Postgres) ColumnsQuery() string {
	return `SELECT column_name, data_type, COALESCE(character_maximum_length, 0), is_nullable = 'YES' ` +
		`FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1`
}

func (Postgres) UniqueColumnsQuery() string {
	return `SELECT a.attname FROM pg_index i ` +
		`JOIN pg_class t ON t.oid = i.indrelid ` +
		`JOIN pg_namespace n ON n.oid = t.relnamespace ` +
		`JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = i.indkey[0] ` +
		`WHERE i.indisunique AND i.indnatts = 1 AND i.indpred IS NULL AND n.nspname = current_schema() AND t.relname = $1`
}

func (Postgres) ForeignKeysQuery() string {
	return `SELECT a.attname, rt.relname, CASE c.confdeltype WHEN 'c' THEN 'CASCADE' WHEN 'r' THEN 'RESTRICT' ` +
		`WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT' ELSE 'NO ACTION' END ` +
		`FROM pg_constraint c ` +
		`JOIN pg_class t ON t.oid = c.conrelid ` +
		`JOIN pg_namespace n ON n.oid = t.relnamespace ` +
		`JOIN pg_class rt ON rt.oid = c.confrelid ` +
		`JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = c.conkey[1] ` +
		`WHERE c.contype = 'f' AND array_length(c.conkey, 1) = 1 AND n.nspname = current_schema() AND t.relname = $1`
}

func (Postgres) IndexesQuery() string {
	return `SELECT indexname FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1`
}

func (Postgres) ForeignKeysEnforcedQuery() string { return "" }

var (
	regexpPostgresCharType    = regexp.MustCompile(`^(VARCHAR|CHARACTER VARYING|BPCHAR|CHAR|CHARACTER)\(([0-9]+)\)$`)
	regexpPostgresNumericType = regexp.MustCompile(`^(NUMERIC|DECIMAL)(\([0-9]+(,[0-9]+)?\))?$`)
)

// StoredType returns the type as in information_schema.columns. The precision and scale of numeric are not compared.
func (Postgres) StoredType(columnType string) (string, int) {
	columnType = strings.ToUpper(strings.TrimSpace(columnType))

	if regexpPostgresNumericType.MatchString(columnType) {
		return "numeric", 0
	}

	if matches := regexpPostgresCharType.FindStringSubmatch(columnType); matches != nil {
		length, _ := strconv.Atoi(matches[2])
		if matches[1] == "VARCHAR" || matches[1] == "CHARACTER VARYING" {
			return "character varying", length
		}
		return "character", length
	}

	switch columnType {
	case "TEXT":
		return "text", 0
	case "BPCHAR":
		return "character", 0
	case "BOOLEAN":
		return "boolean", 0
	case "BIGINT":
		return "bigint", 0
	case "INTEGER":
		return "integer", 0
	case "SMALLINT":
		return "smallint", 0
	case "DOUBLE PRECISION":
		return "double precision", 0
	case "REAL":
		return "real", 0
	case "TIMESTAMPTZ":
		return "timestamp with time zone", 0
	case "UUID":
		return "uuid", 0
	case "BYTEA":
		return "bytea", 0
	case "JSONB":
		return "jsonb", 0
	case "JSON":
		return "json", 0
	case "DATE":
		return "date", 0
	default:
		return "", 0
	}
}

func (Postgres) StoredUUIDType() string { return "uuid" }

// postgresMaxIdentifierLength is the maximum length of a name in PostgreSQL, longer names are truncated.
const postgresMaxIdentifierLength = 63

func (Postgres) StoredIndexName(name string) string {
	if len(name) > postgresMaxIdentifierLength {
		return name[:postgresMaxIdentifierLength]
	}
	return name
}

func (p Postgres) UniqueKey(table string, name string, columns []string) string {
	if name != "" {
		return p.StoredIndexName(name)
	}
	return constraintName(table, columns[0])
}

// constraintName returns the name that PostgreSQL gives the UNIQUE constraint of a column, e.g. "user_email_key", in
// the way of its makeObjectName: the longer of the names of the table and the column is shortened, until the name
// fits in 63 bytes.
func constraintName(table string, column string) string {
	const suffix = "_key"
	tableLength, columnLength := len(table), len(column)
	for tableLength+1+columnLength+len(suffix) > postgresMaxIdentifierLength {
		if tableLength > columnLength {
			tableLength--
		} else {
			columnLength--
		}
	}
	return clipName(table, tableLength) + "_" + clipName(column, columnLength) + suffix
}

// clipName returns the first length bytes of name, without the bytes of a character that does not fit.
func clipName(name string, length int) string {
	for length > 0 && length < len(name) && !utf8.RuneStart(name[length]) {
		length--
	}
	return name[:length]
}
