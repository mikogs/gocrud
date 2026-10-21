package gocrud

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mikogs/gocrud/pkg/sql/builder/querycontainer"
)

// AliasedColumnsMigrateError is returned by Migrate for a struct with aliased column names, which reads from
// several tables.
var AliasedColumnsMigrateError = errors.New("struct with aliased column names cannot be migrated")

// postgresMaxIdentifierLength is the maximum length of a name in PostgreSQL, longer names are truncated.
const postgresMaxIdentifierLength = 63

var regexpPostgresCharType = regexp.MustCompile(`^(VARCHAR|CHARACTER VARYING|BPCHAR|CHAR|CHARACTER)\(([0-9]+)\)$`)

// Migration describes the changes made by Migrate.
type Migration struct {
	// Table is the name of the table.
	Table string
	// CreatedTable is set to true when the table did not exist and was created.
	CreatedTable bool
	// AddedColumns contains the names of the columns added to the table.
	AddedColumns []string
	// CreatedIndexes contains the names of the indexes created on the table.
	CreatedIndexes []string
	// Differences describes the differences between the struct and the table that Migrate does not change,
	// because changing them could lose data: columns that are not fields of the struct, columns of a different
	// type or nullability, existing columns of uniq fields that are not unique, and indexes created by gocrud
	// that are no longer declared in the struct.
	Differences []string
}

// databaseColumn describes a column of an existing table.
type databaseColumn struct {
	name     string
	dataType string
	length   int
	nullable bool
}

// Migrate makes the table of obj match the struct, with changes that do not lose data: it creates the table
// when it does not exist, adds the columns of new fields, and creates new indexes. Other differences are
// returned in Migration.Differences. All the changes are made in a transaction.
func (c *CRUD) Migrate(ctx context.Context, obj interface{}) (*Migration, error) {
	builder, err := c.builder(obj)
	if err != nil {
		return nil, getBuilderObjectCRUDError(err)
	}

	if builder.HasAliasedColumnNames() {
		return nil, getCRUDFuncCRUDError("migrate", AliasedColumnsMigrateError)
	}

	migration := &Migration{Table: builder.TableName()}

	err = c.RunInTx(ctx, func(tx *CRUD) error {
		if hasForeignKeys(builder.Columns()) && c.dialect == DialectSQLite {
			var enabled bool
			if err := tx.conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
				return getDBFuncCRUDError("query foreign_keys pragma", err)
			}
			if !enabled {
				migration.Differences = append(migration.Differences, "foreign keys are not enforced, open the database with the _pragma=foreign_keys(1) parameter")
			}
		}

		existingColumns, err := tx.databaseColumns(ctx, migration.Table)
		if err != nil {
			return err
		}

		if len(existingColumns) == 0 {
			migration.CreatedTable = true
			return tx.CreateTable(ctx, obj)
		}

		existingIndexes, err := tx.databaseIndexes(ctx, migration.Table)
		if err != nil {
			return err
		}

		structColumns := map[string]bool{}
		for _, column := range builder.Columns() {
			structColumns[column.Name] = true

			existing, ok := existingColumns[column.Name]
			if ok {
				// the type of the ID column depends on how it was created, e.g. SERIAL is stored as integer
				if !column.IsID {
					migration.Differences = append(migration.Differences, c.columnDifferences(column, existing)...)
				}
				continue
			}

			if column.IsID {
				migration.Differences = append(migration.Differences, fmt.Sprintf(`column "%s" of the ID field does not exist`, column.Name))
				continue
			}

			for _, statement := range column.AddStatements {
				if _, err := tx.conn.ExecContext(ctx, statement); err != nil {
					return getDBFuncCRUDError("add column "+column.Name, err)
				}
			}
			migration.AddedColumns = append(migration.AddedColumns, column.Name)
		}

		for _, existing := range existingColumns {
			if !structColumns[existing.name] {
				migration.Differences = append(migration.Differences, fmt.Sprintf(`column "%s" is not a field of the struct`, existing.name))
			}
		}

		declaredIndexes := map[string]bool{}
		for i, statement := range builder.IndexStatements() {
			name := c.storedIndexName(builder.IndexNames()[i])
			declaredIndexes[name] = true

			if existingIndexes[name] {
				continue
			}

			if _, err := tx.conn.ExecContext(ctx, statement); err != nil {
				return getDBFuncCRUDError("create index "+builder.IndexNames()[i], err)
			}
			migration.CreatedIndexes = append(migration.CreatedIndexes, builder.IndexNames()[i])
		}

		migration.Differences = append(migration.Differences, c.undeclaredIndexes(migration.Table, builder.Columns(), existingIndexes, declaredIndexes)...)

		// columns added above are created unique, so only the existing ones are checked
		uniqueColumns, err := tx.databaseUniqueColumns(ctx, migration.Table)
		if err != nil {
			return err
		}
		for _, column := range builder.Columns() {
			if column.Unique && existingColumns[column.Name].name != "" && !uniqueColumns[column.Name] {
				migration.Differences = append(migration.Differences, fmt.Sprintf(`column "%s" is not unique, the field is tagged with uniq`, column.Name))
			}
		}

		// columns added above are created with their foreign keys, so only the existing ones are checked
		foreignKeys, err := tx.databaseForeignKeys(ctx, migration.Table)
		if err != nil {
			return err
		}
		for _, column := range builder.Columns() {
			if column.ForeignKey != nil && existingColumns[column.Name].name != "" {
				migration.Differences = append(migration.Differences, foreignKeyDifferences(column, foreignKeys[column.Name])...)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return migration, nil
}

// undeclaredIndexes describes the indexes that were created by gocrud for the table, but are no longer declared in
// the struct. Other indexes, such as the ones created for primary keys and unique constraints, are not checked.
func (c *CRUD) undeclaredIndexes(table string, columns []querycontainer.Column, existingIndexes map[string]bool, declaredIndexes map[string]bool) []string {
	// SQLite unique columns added by Migrate have a unique index instead of a constraint
	for _, column := range columns {
		if column.Unique {
			declaredIndexes[c.storedIndexName(fmt.Sprintf("uniq_%s_%s", table, column.Name))] = true
		}
	}

	var differences []string
	for name := range existingIndexes {
		if declaredIndexes[name] {
			continue
		}

		if strings.HasPrefix(name, c.storedIndexName("idx_"+table+"_")) || strings.HasPrefix(name, c.storedIndexName("uniq_"+table+"_")) {
			differences = append(differences, fmt.Sprintf(`index "%s" is not declared in the struct`, name))
		}
	}
	sort.Strings(differences)

	return differences
}

// storedIndexName returns the name of an index as the database stores it. PostgreSQL truncates names to 63 bytes.
func (c *CRUD) storedIndexName(name string) string {
	if c.dialect == DialectPostgres && len(name) > postgresMaxIdentifierLength {
		return name[:postgresMaxIdentifierLength]
	}

	return name
}

// databaseColumns returns the columns of the table, or no columns when the table does not exist.
func (c *CRUD) databaseColumns(ctx context.Context, table string) (map[string]databaseColumn, error) {
	query := `SELECT column_name, data_type, COALESCE(character_maximum_length, 0), is_nullable = 'YES' ` +
		`FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1`
	if c.dialect == DialectSQLite {
		query = `SELECT name, type, 0, "notnull" = 0 AND pk = 0 FROM pragma_table_info(?)`
	}

	rows, err := c.conn.QueryContext(ctx, query, table)
	if err != nil {
		return nil, getDBFuncCRUDError("query table columns", err)
	}
	defer rows.Close()

	columns := map[string]databaseColumn{}
	for rows.Next() {
		var column databaseColumn
		if err := rows.Scan(&column.name, &column.dataType, &column.length, &column.nullable); err != nil {
			return nil, getDBFuncCRUDError("scan table columns", err)
		}
		columns[column.name] = column
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("query table columns", err)
	}

	return columns, nil
}

// databaseUniqueColumns returns the names of the columns that are unique on their own: they have a unique index
// on the column only, which is also how UNIQUE constraints are implemented. Partial indexes are not included.
func (c *CRUD) databaseUniqueColumns(ctx context.Context, table string) (map[string]bool, error) {
	query := `SELECT a.attname FROM pg_index i ` +
		`JOIN pg_class t ON t.oid = i.indrelid ` +
		`JOIN pg_namespace n ON n.oid = t.relnamespace ` +
		`JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = i.indkey[0] ` +
		`WHERE i.indisunique AND i.indnatts = 1 AND i.indpred IS NULL AND n.nspname = current_schema() AND t.relname = $1`
	if c.dialect == DialectSQLite {
		query = `SELECT ii.name FROM pragma_index_list(?) AS il, pragma_index_info(il.name) AS ii ` +
			`WHERE il."unique" = 1 AND il.partial = 0 AND (SELECT COUNT(*) FROM pragma_index_info(il.name)) = 1`
	}

	rows, err := c.conn.QueryContext(ctx, query, table)
	if err != nil {
		return nil, getDBFuncCRUDError("query unique columns", err)
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, getDBFuncCRUDError("scan unique columns", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("query unique columns", err)
	}

	return columns, nil
}

// databaseForeignKeys returns the foreign keys of single columns of the table, by the column name.
func (c *CRUD) databaseForeignKeys(ctx context.Context, table string) (map[string]*querycontainer.ForeignKey, error) {
	query := `SELECT a.attname, rt.relname, CASE c.confdeltype WHEN 'c' THEN 'CASCADE' WHEN 'r' THEN 'RESTRICT' ` +
		`WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT' ELSE 'NO ACTION' END ` +
		`FROM pg_constraint c ` +
		`JOIN pg_class t ON t.oid = c.conrelid ` +
		`JOIN pg_namespace n ON n.oid = t.relnamespace ` +
		`JOIN pg_class rt ON rt.oid = c.confrelid ` +
		`JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = c.conkey[1] ` +
		`WHERE c.contype = 'f' AND array_length(c.conkey, 1) = 1 AND n.nspname = current_schema() AND t.relname = $1`
	if c.dialect == DialectSQLite {
		query = `SELECT "from", "table", on_delete FROM pragma_foreign_key_list(?) GROUP BY id HAVING COUNT(*) = 1`
	}

	rows, err := c.conn.QueryContext(ctx, query, table)
	if err != nil {
		return nil, getDBFuncCRUDError("query foreign keys", err)
	}
	defer rows.Close()

	foreignKeys := map[string]*querycontainer.ForeignKey{}
	for rows.Next() {
		var column string
		foreignKey := &querycontainer.ForeignKey{}
		if err := rows.Scan(&column, &foreignKey.Table, &foreignKey.OnDelete); err != nil {
			return nil, getDBFuncCRUDError("scan foreign keys", err)
		}
		foreignKeys[column] = foreignKey
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("query foreign keys", err)
	}

	return foreignKeys, nil
}

// foreignKeyDifferences describes how the foreign key of an existing column differs from the one of a struct field.
func foreignKeyDifferences(column querycontainer.Column, existing *querycontainer.ForeignKey) []string {
	switch {
	case existing == nil:
		return []string{fmt.Sprintf(`column "%s" has no foreign key, the field references table "%s"`, column.Name, column.ForeignKey.Table)}
	case existing.Table != column.ForeignKey.Table:
		return []string{fmt.Sprintf(`column "%s" references table "%s", the field references table "%s"`, column.Name, existing.Table, column.ForeignKey.Table)}
	case !strings.EqualFold(existing.OnDelete, column.ForeignKey.OnDelete):
		return []string{fmt.Sprintf(`foreign key of column "%s" has ON DELETE %s, the field needs %s`, column.Name, strings.ToUpper(existing.OnDelete), column.ForeignKey.OnDelete)}
	default:
		return nil
	}
}

func hasForeignKeys(columns []querycontainer.Column) bool {
	for _, column := range columns {
		if column.ForeignKey != nil {
			return true
		}
	}

	return false
}

// databaseIndexes returns the names of the indexes of the table.
func (c *CRUD) databaseIndexes(ctx context.Context, table string) (map[string]bool, error) {
	query := `SELECT indexname FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1`
	if c.dialect == DialectSQLite {
		query = `SELECT name FROM pragma_index_list(?)`
	}

	rows, err := c.conn.QueryContext(ctx, query, table)
	if err != nil {
		return nil, getDBFuncCRUDError("query table indexes", err)
	}
	defer rows.Close()

	indexes := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, getDBFuncCRUDError("scan table indexes", err)
		}
		indexes[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("query table indexes", err)
	}

	return indexes, nil
}

// columnDifferences describes how an existing column differs from the column of a struct field.
func (c *CRUD) columnDifferences(column querycontainer.Column, existing databaseColumn) []string {
	var differences []string

	wantType, wantLength := column.Type, 0
	if c.dialect == DialectPostgres {
		wantType, wantLength = postgresDataType(column.Type)
	}

	// a type that is not known is not compared
	if wantType != "" && (!strings.EqualFold(wantType, existing.dataType) || wantLength != existing.length) {
		differences = append(differences, fmt.Sprintf(`column "%s" has type %s, the field needs %s`,
			column.Name, typeWithLength(existing.dataType, existing.length), typeWithLength(wantType, wantLength)))
	}

	if column.Nullable && !existing.nullable {
		differences = append(differences, fmt.Sprintf(`column "%s" is NOT NULL, the field is nullable`, column.Name))
	}
	if !column.Nullable && existing.nullable {
		differences = append(differences, fmt.Sprintf(`column "%s" is nullable, the field is not`, column.Name))
	}

	return differences
}

// postgresDataType returns the name of a column type as in information_schema.columns, with its length.
// An empty name is returned for a type that is not known.
func postgresDataType(columnType string) (string, int) {
	columnType = strings.ToUpper(strings.TrimSpace(columnType))

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
	default:
		return "", 0
	}
}

func typeWithLength(dataType string, length int) string {
	if length > 0 {
		return fmt.Sprintf("%s(%d)", dataType, length)
	}
	return dataType
}

// Migrate makes the table of objects of type T match the struct, see CRUD.Migrate.
func Migrate[T any](ctx context.Context, c *CRUD) (*Migration, error) {
	return c.Migrate(ctx, new(T))
}
