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

// ErrAliasedColumnsMigrate is returned by Migrate for a struct with aliased column names, which reads from
// several tables.
var ErrAliasedColumnsMigrate = errors.New("struct with aliased column names cannot be migrated")

// ErrSchemaDiffers is matched by the error of CheckSchema when the tables do not match the structs.
var ErrSchemaDiffers = errors.New("database schema differs from the structs")

// SchemaError is returned by CheckSchema. It describes the tables that do not match the structs, with the changes
// that Migrate would make, and the differences that it would report.
type SchemaError struct {
	Migrations []*Migration
}

func (e *SchemaError) Error() string {
	var details []string
	for _, migration := range e.Migrations {
		for _, statement := range migration.Statements {
			details = append(details, fmt.Sprintf(`table "%s": %s`, migration.Table, statement))
		}
		for _, difference := range migration.Differences {
			details = append(details, fmt.Sprintf(`table "%s": %s`, migration.Table, difference))
		}
	}

	return ErrSchemaDiffers.Error() + ": " + strings.Join(details, "; ")
}

func (e *SchemaError) Unwrap() error {
	return ErrSchemaDiffers
}

// MigrationSQL returns the statements of the migrations as an SQL script, e.g. for the file of a migration tool. The
// differences, which have to be resolved by hand, are added as comments. Migrations without statements and
// differences are left out, so an empty string means that the tables match the structs.
func MigrationSQL(migrations ...*Migration) string {
	var b strings.Builder
	for _, migration := range migrations {
		if len(migration.Statements) == 0 && len(migration.Differences) == 0 {
			continue
		}

		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "-- table \"%s\"\n", migration.Table)
		for _, statement := range migration.Statements {
			b.WriteString(statement + ";\n")
		}
		for _, difference := range migration.Differences {
			b.WriteString("-- TODO: " + difference + "\n")
		}
	}

	return b.String()
}

// newSchemaError returns a *SchemaError for the migrations that have statements or differences, or nil when there
// are none.
func newSchemaError(migrations []*Migration) error {
	var differing []*Migration
	for _, migration := range migrations {
		if len(migration.Statements) > 0 || len(migration.Differences) > 0 {
			differing = append(differing, migration)
		}
	}

	if len(differing) == 0 {
		return nil
	}
	return &SchemaError{Migrations: differing}
}

// CheckSchema checks that the tables of objs match the structs: that Migrate would not change them, and would not
// report differences. Otherwise, it returns a *SchemaError, which matches ErrSchemaDiffers. Use it in a test, or at
// the start of an application, when the tables are changed by a migration tool instead of Migrate.
func (c *CRUD) CheckSchema(ctx context.Context, objs ...interface{}) error {
	migrations := make([]*Migration, 0, len(objs))
	for _, obj := range objs {
		migration, err := c.PlanMigration(ctx, obj)
		if err != nil {
			return err
		}
		migrations = append(migrations, migration)
	}

	return newSchemaError(migrations)
}

// postgresMaxIdentifierLength is the maximum length of a name in PostgreSQL, longer names are truncated.
const postgresMaxIdentifierLength = 63

var regexpPostgresCharType = regexp.MustCompile(`^(VARCHAR|CHARACTER VARYING|BPCHAR|CHAR|CHARACTER)\(([0-9]+)\)$`)

// Migration describes the changes that Migrate makes to a table, or that PlanMigration plans.
type Migration struct {
	// Table is the name of the table.
	Table string
	// CreatedTable is set to true when the table does not exist, and is created.
	CreatedTable bool
	// AddedColumns contains the names of the columns that are added to the table.
	AddedColumns []string
	// CreatedIndexes contains the names of the indexes that are created on the table.
	CreatedIndexes []string
	// Statements contains the SQL statements that make the changes, in the order in which they are run.
	Statements []string
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
	var migration *Migration
	err := c.RunInTx(ctx, func(tx *CRUD) error {
		var err error
		migration, err = tx.PlanMigration(ctx, obj)
		if err != nil {
			return err
		}

		for _, statement := range migration.Statements {
			if _, err := tx.conn.ExecContext(ctx, statement); err != nil {
				return getDBFuncCRUDError("migrate "+migration.Table, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return migration, nil
}

// PlanMigration returns the changes that Migrate would make to the table of obj, without making them, e.g. to write
// them to a file of a migration tool. The statements are planned for the database as it is, so they have to be run
// in their order, and before the database changes in another way.
func (c *CRUD) PlanMigration(ctx context.Context, obj interface{}) (*Migration, error) {
	builder, err := c.builder(obj)
	if err != nil {
		return nil, getBuilderObjectCRUDError(err)
	}

	if builder.HasAliasedColumnNames() {
		return nil, getCRUDFuncCRUDError("migrate", ErrAliasedColumnsMigrate)
	}

	migration := &Migration{Table: builder.TableName()}

	if hasForeignKeys(builder.Columns()) && c.dialect == DialectSQLite {
		var enabled bool
		if err := c.conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
			return nil, getDBFuncCRUDError("query foreign_keys pragma", err)
		}
		if !enabled {
			migration.Differences = append(migration.Differences, "foreign keys are not enforced, open the database with the _pragma=foreign_keys(1) parameter")
		}
	}

	existingColumns, err := c.databaseColumns(ctx, migration.Table)
	if err != nil {
		return nil, err
	}

	if len(existingColumns) == 0 {
		migration.CreatedTable = true
		migration.Statements, err = createTableStatements(obj, builder)
		if err != nil {
			return nil, err
		}
		return migration, nil
	}

	existingIndexes, err := c.databaseIndexes(ctx, migration.Table)
	if err != nil {
		return nil, err
	}

	structColumns := map[string]bool{}
	for _, column := range builder.Columns() {
		structColumns[column.Name] = true

		existing, ok := existingColumns[column.Name]
		if ok {
			if column.IsID {
				migration.Differences = append(migration.Differences, c.idColumnDifferences(builder.HasUUID(), existing)...)
			} else {
				migration.Differences = append(migration.Differences, c.columnDifferences(column, existing)...)
			}
			continue
		}

		if column.IsID {
			migration.Differences = append(migration.Differences, fmt.Sprintf(`column "%s" of the ID field does not exist`, column.Name))
			continue
		}

		migration.Statements = append(migration.Statements, column.AddStatements...)
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

		migration.Statements = append(migration.Statements, statement)
		migration.CreatedIndexes = append(migration.CreatedIndexes, builder.IndexNames()[i])
	}

	migration.Differences = append(migration.Differences, c.undeclaredIndexes(migration.Table, builder.Columns(), existingIndexes, declaredIndexes)...)

	// added columns are created unique, so only the existing ones are checked
	uniqueColumns, err := c.databaseUniqueColumns(ctx, migration.Table)
	if err != nil {
		return nil, err
	}
	for _, column := range builder.Columns() {
		if column.Unique && existingColumns[column.Name].name != "" && !uniqueColumns[column.Name] {
			migration.Differences = append(migration.Differences, fmt.Sprintf(`column "%s" is not unique, the field is tagged with uniq`, column.Name))
		}
	}

	// added columns are created with their foreign keys, so only the existing ones are checked
	foreignKeys, err := c.databaseForeignKeys(ctx, migration.Table)
	if err != nil {
		return nil, err
	}
	for _, column := range builder.Columns() {
		if column.ForeignKey != nil && existingColumns[column.Name].name != "" {
			migration.Differences = append(migration.Differences, foreignKeyDifferences(column, foreignKeys[column.Name])...)
		}
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

// idColumnDifferences describes how the column of the ID differs from the ID field: a UUID is stored as uuid in
// PostgreSQL and as TEXT in SQLite. The type of a numeric ID column depends on how it was created, e.g. SERIAL is
// stored as integer, so it is not compared otherwise.
func (c *CRUD) idColumnDifferences(isUUID bool, existing databaseColumn) []string {
	uuidType := "uuid"
	if c.dialect == DialectSQLite {
		uuidType = "TEXT"
	}

	switch {
	case isUUID && !strings.EqualFold(existing.dataType, uuidType):
		return []string{fmt.Sprintf(`column "%s" has type %s, the ID field is a string with a UUID`, existing.name, existing.dataType)}
	case !isUUID && strings.EqualFold(existing.dataType, uuidType):
		return []string{fmt.Sprintf(`column "%s" has type %s, the ID field is a uint64`, existing.name, existing.dataType)}
	default:
		return nil
	}
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
	case "UUID":
		return "uuid", 0
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

// PlanMigration returns the changes that Migrate would make to the table of objects of type T, see
// CRUD.PlanMigration.
func PlanMigration[T any](ctx context.Context, c *CRUD) (*Migration, error) {
	return c.PlanMigration(ctx, new(T))
}
