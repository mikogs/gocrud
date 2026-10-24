package gocrud

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
)

// ErrAliasedColumnsMigrate is returned by Migrate for a struct with aliased column names, which reads from
// several tables.
var ErrAliasedColumnsMigrate = errors.New("struct with aliased column names cannot be migrated")

// Migration describes the changes that Migrate makes to a table, or that PlanMigration plans.
type Migration struct {
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

		migrateCtx := c.withQuery(ctx, "Migrate", migration.Table)
		for _, statement := range migration.Statements {
			if _, err := tx.conn.ExecContext(migrateCtx, statement); err != nil {
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
	ctx = c.withQuery(ctx, "PlanMigration", builder.TableName())

	if builder.HasAliasedColumnNames() {
		return nil, getCRUDFuncCRUDError("migrate", ErrAliasedColumnsMigrate)
	}

	migration := &Migration{Table: builder.TableName()}

	if query := c.sqlDialect.ForeignKeysEnforcedQuery(); query != "" && hasForeignKeys(builder.Columns()) {
		var enabled bool
		if err := c.conn.QueryRowContext(ctx, query).Scan(&enabled); err != nil {
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

// columnDifferences describes how an existing column differs from the column of a struct field.
func (c *CRUD) columnDifferences(column sqlbuilder.Column, existing databaseColumn) []string {
	var differences []string

	wantType, wantLength := c.sqlDialect.StoredType(column.Type)

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
	uuidType := c.sqlDialect.StoredUUIDType()

	switch {
	case isUUID && !strings.EqualFold(existing.dataType, uuidType):
		return []string{fmt.Sprintf(`column "%s" has type %s, the ID field is a string with a UUID`, existing.name, existing.dataType)}
	case !isUUID && strings.EqualFold(existing.dataType, uuidType):
		return []string{fmt.Sprintf(`column "%s" has type %s, the ID field is a uint64`, existing.name, existing.dataType)}
	default:
		return nil
	}
}

// typeWithLength returns a type with its length, for the descriptions of the differences of columns.
func typeWithLength(dataType string, length int) string {
	if length > 0 {
		return fmt.Sprintf("%s(%d)", dataType, length)
	}
	return dataType
}

// foreignKeyDifferences describes how the foreign key of an existing column differs from the one of a struct field.
func foreignKeyDifferences(column sqlbuilder.Column, existing *sqlbuilder.ForeignKey) []string {
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

func hasForeignKeys(columns []sqlbuilder.Column) bool {
	for _, column := range columns {
		if column.ForeignKey != nil {
			return true
		}
	}

	return false
}

// undeclaredIndexes describes the indexes that were created by gocrud for the table, but are no longer declared in
// the struct. Other indexes, such as the ones created for primary keys and unique constraints, are not checked.
func (c *CRUD) undeclaredIndexes(table string, columns []sqlbuilder.Column, existingIndexes map[string]bool, declaredIndexes map[string]bool) []string {
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

// storedIndexName returns the name of an index as the database stores it, e.g. PostgreSQL truncates names to 63 bytes.
func (c *CRUD) storedIndexName(name string) string {
	return c.sqlDialect.StoredIndexName(name)
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
