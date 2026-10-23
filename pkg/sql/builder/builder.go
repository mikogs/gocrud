package builder

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/mikogs/gocrud/pkg/filters"
	postgresQueryBuilder "github.com/mikogs/gocrud/pkg/sql/builder/querybuilder/postgres"
	sqliteQueryBuilder "github.com/mikogs/gocrud/pkg/sql/builder/querybuilder/sqlite"
	"github.com/mikogs/gocrud/pkg/sql/builder/querybuilder/where"
	"github.com/mikogs/gocrud/pkg/sql/builder/querycontainer"
	postgresQueryContainer "github.com/mikogs/gocrud/pkg/sql/builder/querycontainer/postgres"
	sqliteQueryContainer "github.com/mikogs/gocrud/pkg/sql/builder/querycontainer/sqlite"
	"github.com/mikogs/gocrud/pkg/structinfo"
)

const DefaultTagName = "crud"

type dynamicQueryBuilder interface {
	Select(order []string, limit int, offset int, filters filters.Filter) (string, error)
	SelectCount(filters filters.Filter) (string, error)
	Delete(filters filters.Filter) (string, error)
	DeleteReturningID(filters filters.Filter) (string, error)
	Update(values map[string]interface{}, filters filters.Filter) (string, error)
	WhereDialect(qualifier string, withAlias bool, placeholder func() string) where.Dialect
}

// Builder reflects the object to generate and cache SQL queries (CREATE TABLE, INSERT, UPDATE, etc.).
// Database table and column names are lowercase with an underscore, and they are generated from field names.
type Builder struct {
	tagName    string
	structInfo *structinfo.StructInfo

	tableName              string
	fieldColumns           map[string]string
	relatedBuilder         func(t reflect.Type) (*Builder, error)
	columns                []querycontainer.Column
	indexStatements        []string
	indexNames             []string
	dropTable              string
	createTable            string
	insert                 string
	insertOnConflictUpdate string
	updateByID             string
	selectByID             string
	deleteByID             string

	queryBuilder dynamicQueryBuilder
}

// New takes a pointer to a struct and returns a Builder instance.
// It returns an error when the struct cannot be mapped to a database table.
func New(obj interface{}, options Options) (*Builder, error) {
	b := &Builder{}

	b.relatedBuilder = options.RelatedBuilder

	b.tagName = DefaultTagName
	if options.TagName != "" {
		b.tagName = options.TagName
	}

	var err error
	b.structInfo, err = structinfo.New(obj, b.tagName)
	if err != nil {
		return nil, err
	}

	switch options.Dialect {
	case DialectSQLite:
		container, err := sqliteQueryContainer.New(obj, b.structInfo, options.TableNamePrefix)
		if err != nil {
			return nil, err
		}
		b.tableName = container.RawTableName
		b.fieldColumns = container.FieldColumnName
		b.columns = container.Columns
		b.indexStatements = container.IndexStatements
		b.indexNames = container.IndexNames
		b.dropTable = container.DropTable
		b.createTable = container.CreateTable
		b.insert = container.Insert
		b.insertOnConflictUpdate = container.InsertOnConflictUpdate
		b.updateByID = container.UpdateByID
		b.selectByID = container.SelectByID
		b.deleteByID = container.DeleteByID
		queryBuilder := sqliteQueryBuilder.New(b.structInfo, container)
		queryBuilder.Relation = b.relation
		queryBuilder.Subquery = b.subquery
		b.queryBuilder = queryBuilder
	default: // postgres
		container, err := postgresQueryContainer.New(obj, b.structInfo, options.TableNamePrefix)
		if err != nil {
			return nil, err
		}
		b.tableName = container.RawTableName
		b.fieldColumns = container.FieldColumnName
		b.columns = container.Columns
		b.indexStatements = container.IndexStatements
		b.indexNames = container.IndexNames
		b.dropTable = container.DropTable
		b.createTable = container.CreateTable
		b.insert = container.Insert
		b.insertOnConflictUpdate = container.InsertOnConflictUpdate
		b.updateByID = container.UpdateByID
		b.selectByID = container.SelectByID
		b.deleteByID = container.DeleteByID
		queryBuilder := postgresQueryBuilder.New(b.structInfo, container)
		queryBuilder.Relation = b.relation
		queryBuilder.Subquery = b.subquery
		b.queryBuilder = queryBuilder
	}

	return b, nil
}

// relation returns the rows of the related struct of a relation field, for where.Dialect.Relation, with the columns of
// the related struct qualified with alias, and the columns of this struct qualified with outer.
func (b *Builder) relation(outer string, field string, alias string, placeholder func() string) (where.Related, error) {
	var relation *structinfo.RelationInfo
	for _, r := range b.structInfo.Relations {
		if r.Field == field {
			relation = r
		}
	}
	if relation == nil {
		return where.Related{}, fmt.Errorf("%w: %s is not a relation field of %s", where.ErrRelationPath, field, b.structInfo.Name)
	}
	if b.relatedBuilder == nil {
		return where.Related{}, fmt.Errorf("%w: %s: related structs are not available", where.ErrRelationPath, field)
	}

	related, err := b.relatedBuilder(relation.Type)
	if err != nil {
		return where.Related{}, fmt.Errorf("%s: %w", field, err)
	}

	quote := func(column string) string { return `"` + column + `"` }
	var match string
	if relation.Kind == structinfo.BelongsTo {
		// the related row has the ID that is in the key field of this row
		match = alias + "." + quote(related.fieldColumns["ID"]) + " = " + outer + "." + quote(b.fieldColumns[relation.KeyField])
	} else {
		// the related rows have the ID of this row in their key field
		match = alias + "." + quote(related.fieldColumns[relation.KeyField]) + " = " + outer + "." + quote(b.fieldColumns["ID"])
	}

	return where.Related{
		Dialect: related.queryBuilder.WhereDialect(alias, false, placeholder),
		Table:   quote(related.tableName),
		Match:   match,
		Many:    relation.Kind == structinfo.HasMany,
	}, nil
}

// subquery returns the rows of the struct of obj, for where.Dialect.Subquery, with its columns qualified with alias.
func (b *Builder) subquery(obj interface{}, field string, alias string, placeholder func() string) (where.Selected, error) {
	if b.relatedBuilder == nil {
		return where.Selected{}, fmt.Errorf("%w: other structs are not available", where.ErrSubquery)
	}

	t := reflect.TypeOf(obj)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return where.Selected{}, fmt.Errorf("%w: %T is not a struct", where.ErrSubquery, obj)
	}

	sub, err := b.relatedBuilder(t)
	if err != nil {
		return where.Selected{}, fmt.Errorf("%w: %s: %w", where.ErrSubquery, t.Name(), err)
	}

	if !slices.Contains(sub.ColumnFieldNames(), field) {
		return where.Selected{}, fmt.Errorf("%w: %s is not a stored field of %s", where.ErrSubquery, field, t.Name())
	}
	if slices.Contains(sub.PasswordFields(), field) {
		return where.Selected{}, fmt.Errorf("%w: %s of %s", where.ErrPasswordField, field, t.Name())
	}

	quote := func(column string) string { return `"` + column + `"` }
	return where.Selected{
		Dialect: sub.queryBuilder.WhereDialect(alias, false, placeholder),
		Table:   quote(sub.tableName),
		Column:  alias + "." + quote(sub.fieldColumns[field]),
	}, nil
}

// WhereDialect returns the dialect of conditions on the fields of the struct, with the columns qualified with
// qualifier unless it is empty, e.g. for a query that joins other tables.
func (b *Builder) WhereDialect(qualifier string, placeholder func() string) where.Dialect {
	return b.queryBuilder.WhereDialect(qualifier, false, placeholder)
}

// QuotedTableName returns the quoted name of the table.
func (b *Builder) QuotedTableName() string { return `"` + b.tableName + `"` }

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

// HasModificationFields returns true if all the following int64 fields are present: CreatedAt, CreatedBy, ModifiedAt, ModifiedBy.
func (b *Builder) HasModificationFields() bool { return b.structInfo.ModificationFields }

// HasUUID returns true if the ID field is a string with a UUID.
func (b *Builder) HasUUID() bool { return b.structInfo.UUID }

// UUIDFields returns the names of the fields that hold UUIDs: the ID with a UUID, and the string foreign keys.
func (b *Builder) UUIDFields() []string { return b.structInfo.UUIDFields }

// HasAliasedColumnNames returns true if any of the fields have column names in the format of "alias.column_name".
func (b *Builder) HasAliasedColumnNames() bool { return b.structInfo.AliasedColumnNames }

// ColumnFieldNames returns an ordered list of names of the fields that are stored in the database.
// It is the order of columns in the generated queries.
func (b *Builder) ColumnFieldNames() []string { return b.structInfo.ColumnFieldNames }

// UpdateFieldNames returns an ordered list of names of the fields that are written when an object is updated.
// It is the order of columns in the SET clause of the generated UPDATE BY ID and INSERT ON CONFLICT UPDATE queries.
func (b *Builder) UpdateFieldNames() []string { return b.structInfo.UpdateFieldNames }

// TimeFields returns a list with names of the time.Time fields that are stored in the database.
func (b *Builder) TimeFields() []string { return b.structInfo.TimeFields }

// StructName returns the name of the struct type.
func (b *Builder) StructName() string { return b.structInfo.Name }

// TableName returns the name of the table, without quotes.
func (b *Builder) TableName() string { return b.tableName }

// Columns describes the columns of the table, in the order of the struct fields.
func (b *Builder) Columns() []querycontainer.Column { return b.columns }

// IndexStatements returns a CREATE INDEX IF NOT EXISTS statement for each index declared with 'idx' or 'uidx' tags.
func (b *Builder) IndexStatements() []string { return b.indexStatements }

// IndexNames returns the names of the indexes created by IndexStatements.
func (b *Builder) IndexNames() []string { return b.indexNames }

// Relations returns the fields that hold related objects, declared with 'rel' tags.
func (b *Builder) Relations() []*structinfo.RelationInfo { return b.structInfo.Relations }

// UniqueFields returns a list with field names that are unique.
func (b *Builder) UniqueFields() []string { return b.structInfo.UniqueFields }

// PasswordFields returns a list with field names that are passwords.
func (b *Builder) PasswordFields() []string { return b.structInfo.PasswordFields }

// Select returns a SELECT query with a WHERE condition built from 'filters'.
func (b *Builder) Select(order []string, limit int, offset int, filters filters.Filter) (string, error) {
	query, err := b.queryBuilder.Select(order, limit, offset, filters)
	if err != nil {
		return "", getQueryBuilderError("Select", err)
	}
	return query + ";", nil
}

// SelectCount returns a SELECT COUNT(*) query to count rows with a WHERE condition built from 'filters'.
func (b *Builder) SelectCount(filters filters.Filter) (string, error) {
	query, err := b.queryBuilder.SelectCount(filters)
	if err != nil {
		return "", getQueryBuilderError("SelectCount", err)
	}
	return query + ";", nil
}

// Delete returns a DELETE query with a WHERE condition built from 'filters'.
func (b *Builder) Delete(filters filters.Filter) (string, error) {
	query, err := b.queryBuilder.Delete(filters)
	if err != nil {
		return "", getQueryBuilderError("Delete", err)
	}
	return query + ";", nil
}

// DeleteReturningID returns a DELETE query with a WHERE condition built from 'filters' with RETURNING id.
func (b *Builder) DeleteReturningID(filters filters.Filter) (string, error) {
	query, err := b.queryBuilder.DeleteReturningID(filters)
	if err != nil {
		return "", getQueryBuilderError("DeleteReturningID", err)
	}
	return query + ";", nil
}

// Update returns an UPDATE query where specified struct fields (columns) are updated and rows match specific WHERE condition built from 'filters'.
func (b *Builder) Update(values map[string]interface{}, filters filters.Filter) (string, error) {
	query, err := b.queryBuilder.Update(values, filters)
	if err != nil {
		return "", getQueryBuilderError("Update", err)
	}
	return query + ";", nil
}
