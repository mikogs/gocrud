package builder

import (
	"github.com/mikolajgasior/gocrud/pkg/filters"
	postgresQueryBuilder "github.com/mikolajgasior/gocrud/pkg/sql/builder/querybuilder/postgres"
	sqliteQueryBuilder "github.com/mikolajgasior/gocrud/pkg/sql/builder/querybuilder/sqlite"
	postgresQueryContainer "github.com/mikolajgasior/gocrud/pkg/sql/builder/querycontainer/postgres"
	sqliteQueryContainer "github.com/mikolajgasior/gocrud/pkg/sql/builder/querycontainer/sqlite"
	"github.com/mikolajgasior/gocrud/pkg/structinfo"
)

const DefaultTagName = "sql"

type dynamicQueryBuilder interface {
	Select(order []string, limit int, offset int, filters *filters.Filters) (string, error)
	SelectCount(filters *filters.Filters) (string, error)
	Delete(filters *filters.Filters) (string, error)
	DeleteReturningID(filters *filters.Filters) (string, error)
	Update(values map[string]interface{}, filters *filters.Filters) (string, error)
}

// Builder reflects the object to generate and cache SQL queries (CREATE TABLE, INSERT, UPDATE, etc.).
// Database table and column names are lowercase with an underscore, and they are generated from field names.
type Builder struct {
	tagName    string
	structInfo *structinfo.StructInfo

	columnFieldName        map[string]string
	dropTable              string
	createTable            string
	insert                 string
	insertOnConflictUpdate string
	updateByID             string
	selectByID             string
	deleteByID             string

	queryBuilder dynamicQueryBuilder
}

// New takes a struct and returns a Builder instance.
func New(obj interface{}, options Options) *Builder {
	b := &Builder{}

	b.tagName = DefaultTagName
	if options.TagName != "" {
		b.tagName = options.TagName
	}

	b.structInfo = structinfo.New(obj, b.tagName)

	switch options.Dialect {
	case DialectSQLite:
		container := sqliteQueryContainer.New(obj, b.structInfo, options.TableNamePrefix)
		b.columnFieldName = container.ColumnFieldName
		b.dropTable = container.DropTable
		b.createTable = container.CreateTable
		b.insert = container.Insert
		b.insertOnConflictUpdate = container.InsertOnConflictUpdate
		b.updateByID = container.UpdateByID
		b.selectByID = container.SelectByID
		b.deleteByID = container.DeleteByID
		b.queryBuilder = sqliteQueryBuilder.New(b.structInfo, container)
	default: // postgres
		container := postgresQueryContainer.New(obj, b.structInfo, options.TableNamePrefix)
		b.columnFieldName = container.ColumnFieldName
		b.dropTable = container.DropTable
		b.createTable = container.CreateTable
		b.insert = container.Insert
		b.insertOnConflictUpdate = container.InsertOnConflictUpdate
		b.updateByID = container.UpdateByID
		b.selectByID = container.SelectByID
		b.deleteByID = container.DeleteByID
		b.queryBuilder = postgresQueryBuilder.New(b.structInfo, container)
	}

	return b
}

// DropTable returns an SQL query for dropping the table.
func (b *Builder) DropTable() string { return b.dropTable + ";" }

// CreateTable returns SQL statements for creating the table, separated by "; ".
// When the struct has fields tagged with 'idx' or 'uidx', a CREATE INDEX statement is
// appended for each index after the CREATE TABLE statement.
func (b *Builder) CreateTable() string { return b.createTable + ";" }

// Insert returns an SQL query for inserting a new object to the table.
func (b *Builder) Insert() string { return b.insert + ";" }

// UpdateByID returns an SQL query for updating an object by their ID.
func (b *Builder) UpdateByID() string { return b.updateByID + ";" }

// InsertOnConflictUpdate returns an SQL query for inserting when a conflict is detected.
func (b *Builder) InsertOnConflictUpdate() string { return b.insertOnConflictUpdate + ";" }

// SelectByID returns an SQL query for selecting an object by its ID.
func (b *Builder) SelectByID() string { return b.selectByID + ";" }

// DeleteByID returns an SQL query for deleting an object by its ID.
func (b *Builder) DeleteByID() string { return b.deleteByID + ";" }

// DatabaseColumnToFieldName takes a database column and converts it to a struct field name.
func (b *Builder) DatabaseColumnToFieldName(n string) string { return b.columnFieldName[n] }

// HasModificationFields returns true if all the following int64 fields are present: CreatedAt, CreatedBy, ModifiedAt, ModifiedBy.
func (b *Builder) HasModificationFields() bool { return b.structInfo.ModificationFields }

// HasAliasedColumnNames returns true if any of the fields have column names in the format of "alias.column_name".
func (b *Builder) HasAliasedColumnNames() bool { return b.structInfo.AliasedColumnNames }

// UniqueFields returns a list with field names that are unique.
func (b *Builder) UniqueFields() []string { return b.structInfo.UniqueFields }

// PasswordFields returns a list with field names that are passwords.
func (b *Builder) PasswordFields() []string { return b.structInfo.PasswordFields }

// Select returns a SELECT query with a WHERE condition built from 'filters' (field-value pairs).
func (b *Builder) Select(order []string, limit int, offset int, filters *filters.Filters) (string, error) {
	query, err := b.queryBuilder.Select(order, limit, offset, filters)
	if err != nil {
		return "", getQueryBuilderError("Select", err)
	}
	return query + ";", nil
}

// SelectCount returns a SELECT COUNT(*) query to count rows with a WHERE condition built from 'filters' (field-value pairs).
func (b *Builder) SelectCount(filters *filters.Filters) (string, error) {
	query, err := b.queryBuilder.SelectCount(filters)
	if err != nil {
		return "", getQueryBuilderError("SelectCount", err)
	}
	return query + ";", nil
}

// Delete returns a DELETE query with a WHERE condition built from 'filters' (field-value pairs).
func (b *Builder) Delete(filters *filters.Filters) (string, error) {
	query, err := b.queryBuilder.Delete(filters)
	if err != nil {
		return "", getQueryBuilderError("Delete", err)
	}
	return query + ";", nil
}

// DeleteReturningID returns a DELETE query with a WHERE condition built from 'filters' (field-value pairs) with RETURNING id.
func (b *Builder) DeleteReturningID(filters *filters.Filters) (string, error) {
	query, err := b.queryBuilder.DeleteReturningID(filters)
	if err != nil {
		return "", getQueryBuilderError("DeleteReturningID", err)
	}
	return query + ";", nil
}

// Update returns an UPDATE query where specified struct fields (columns) are updated and rows match specific WHERE condition built from 'filters' (field-value pairs).
func (b *Builder) Update(values map[string]interface{}, filters *filters.Filters) (string, error) {
	query, err := b.queryBuilder.Update(values, filters)
	if err != nil {
		return "", getQueryBuilderError("Update", err)
	}
	return query + ";", nil
}
