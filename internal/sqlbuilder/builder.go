package sqlbuilder

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/mikogs/gocrud/internal/dialect"
	"github.com/mikogs/gocrud/internal/sqlbuilder/where"
	"github.com/mikogs/gocrud/internal/structinfo"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

const DefaultTagName = "crud"

// FieldKind is how the value of a field is read from and written to the database, when database/sql cannot do it on
// its own.
type FieldKind uint8

const (
	FieldDefault FieldKind = iota
	// FieldNamedBool is a field of a named bool type, e.g. type Flag bool, or a pointer to one, which database/sql
	// scans only from bools.
	FieldNamedBool
	// FieldBytes is a field of a []byte type that is not a pointer, which is NOT NULL, while the drivers write nil as
	// NULL.
	FieldBytes
	// FieldJSON is a field stored as JSON.
	FieldJSON
	// FieldValuerPointer is a pointer to a Valuer type, which is written as the value of the Valuer, as the arguments of
	// queries are converted for SQLite.
	FieldValuerPointer
)

// Builder reflects the object to generate and cache SQL queries (CREATE TABLE, INSERT, UPDATE, etc.).
// Database table and column names are lowercase with an underscore, and they are generated from field names.
type Builder struct {
	tagName    string
	structInfo *structinfo.StructInfo
	dialect    dialect.Dialect
	st         *statements

	tableName              string
	fieldColumns           map[string]string
	relatedBuilder         func(t reflect.Type) (*Builder, error)
	columns                []Column
	indexStatements        []string
	indexNames             []string
	uniqueKeyFields        map[string][]string
	dropTable              string
	createTable            string
	insert                 string
	insertOnConflictUpdate string
	updateByID             string
	selectByID             string
	deleteByID             string

	// the indexes of the fields in the struct, so that the fields of objects are not found by name for every row: in
	// the orders of ColumnFieldNames, UpdateFieldNames, TimeFields and PasswordFields
	columnIndexes   []int
	updateIndexes   []int
	timeIndexes     []int
	passwordIndexes []int
	fieldKinds      []FieldKind
	valuerTypes     map[string]reflect.Type
}

// New returns the Builder of obj, a pointer to a struct, or an error when the struct cannot be mapped to a table.
func New(obj any, options Options) (*Builder, error) {
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

	b.dialect = dialect.For(options.Dialect)
	b.st, err = newStatements(obj, b.structInfo, options.TableNamePrefix, b.dialect)
	if err != nil {
		return nil, err
	}

	b.tableName = b.st.RawTableName
	b.fieldColumns = b.st.FieldColumnName
	b.columns = b.st.Columns
	b.indexStatements = b.st.IndexStatements
	b.indexNames = b.st.IndexNames
	b.uniqueKeyFields = b.st.UniqueKeyFields
	b.dropTable = b.st.DropTable
	b.createTable = b.st.CreateTable
	b.insert = b.st.Insert
	b.insertOnConflictUpdate = b.st.InsertOnConflictUpdate
	b.updateByID = b.st.UpdateByID
	b.selectByID = b.st.SelectByID
	b.deleteByID = b.st.DeleteByID

	objType := reflect.TypeOf(obj).Elem()
	indexes := func(names []string) []int {
		list := make([]int, len(names))
		for i, name := range names {
			field, _ := objType.FieldByName(name)
			list[i] = field.Index[0]
		}
		return list
	}
	b.columnIndexes = indexes(b.structInfo.ColumnFieldNames)
	b.updateIndexes = indexes(b.structInfo.UpdateFieldNames)
	b.timeIndexes = indexes(b.structInfo.TimeFields)
	b.passwordIndexes = indexes(b.structInfo.PasswordFields)
	for i, index := range b.columnIndexes {
		if t := structinfo.BaseType(objType.Field(index).Type); structinfo.IsValuerType(t) {
			if b.valuerTypes == nil {
				b.valuerTypes = map[string]reflect.Type{}
			}
			b.valuerTypes[b.structInfo.ColumnFieldNames[i]] = t
		}

		kind := fieldKind(objType.Field(index).Type, b.structInfo.Fields[b.structInfo.ColumnFieldNames[i]].JSON)
		if kind == FieldDefault {
			continue
		}
		if b.fieldKinds == nil {
			b.fieldKinds = make([]FieldKind, objType.NumField())
		}
		b.fieldKinds[index] = kind
	}

	return b, nil
}

func fieldKind(t reflect.Type, isJSON bool) FieldKind {
	switch {
	case isJSON:
		return FieldJSON
	case structinfo.IsNamedBoolType(t):
		return FieldNamedBool
	case structinfo.IsBytesType(t):
		return FieldBytes
	case t.Kind() == reflect.Pointer && structinfo.IsValuerType(t.Elem()):
		return FieldValuerPointer
	default:
		return FieldDefault
	}
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
		Dialect: related.whereDialect(alias, false, placeholder),
		Table:   quote(related.tableName),
		Match:   match,
		Many:    relation.Kind == structinfo.HasMany,
	}, nil
}

// subquery returns the rows of the struct of obj, for where.Dialect.Subquery, with its columns qualified with alias.
func (b *Builder) subquery(obj any, field string, alias string, placeholder func() string) (where.Selected, error) {
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
		Dialect: sub.whereDialect(alias, false, placeholder),
		Table:   quote(sub.tableName),
		Column:  alias + "." + quote(sub.fieldColumns[field]),
	}, nil
}

// WhereDialect returns the dialect of conditions on the fields of the struct, with the columns qualified with
// qualifier unless it is empty, e.g. for a query that joins other tables.
func (b *Builder) WhereDialect(qualifier string, placeholder func() string) where.Dialect {
	return b.whereDialect(qualifier, false, placeholder)
}

func (b *Builder) QuotedTableName() string { return `"` + b.tableName + `"` }

func (b *Builder) DropTable() string { return b.dropTable + ";" }

// CreateTable returns SQL statements for creating the table, separated by "; ".
// When the struct has fields tagged with 'idx' or 'uidx', a CREATE INDEX statement is
// appended for each index after the CREATE TABLE statement.
func (b *Builder) CreateTable() string { return b.createTable + ";" }

func (b *Builder) Insert() string { return b.insert + ";" }

// maxInsertRows limits the rows of an INSERT of InsertRows, so that a query and its values stay small.
const maxInsertRows = 1000

// InsertRowsLimit returns the largest number of rows of an INSERT of InsertRows, which takes the values of the
// inserted columns of each row.
func (b *Builder) InsertRowsLimit() int {
	return min(maxInsertRows, b.dialect.MaxPlaceholders()/len(b.insertColumns()))
}

// InsertRows returns an INSERT of rows rows, which takes the values of the columns without the ID of each row, and
// returns their IDs, or of all the columns for a struct with a UUID, which is generated before.
func (b *Builder) InsertRows(rows int) string {
	columns := b.insertColumns()
	values := make([]string, rows)
	for i := range values {
		values[i] = "(" + placeholders(b.dialect, i*len(columns)+1, len(columns)) + ")"
	}
	query := fmt.Sprintf("INSERT INTO %s(%s) VALUES %s", b.st.TableName, strings.Join(columns, ","), strings.Join(values, ","))
	if !b.structInfo.UUID {
		query += ` RETURNING "id"`
	}
	return query + ";"
}

func (b *Builder) insertColumns() []string {
	if b.structInfo.UUID {
		return b.st.ColumnNames
	}
	return b.st.ColumnNames[1:]
}

func (b *Builder) UpdateByID() string { return b.updateByID + ";" }

func (b *Builder) InsertOnConflictUpdate() string { return b.insertOnConflictUpdate + ";" }

func (b *Builder) SelectByID() string { return b.selectByID + ";" }

func (b *Builder) DeleteByID() string { return b.deleteByID + ";" }

// HasModificationFields returns true if the struct has the fields CreatedAt and ModifiedAt of type time.Time, and
// CreatedBy and ModifiedBy of type uint64 or string.
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

// ColumnIndexes returns the indexes of the fields of ColumnFieldNames in the struct, in the same order, for
// reflect.Value.Field. The first one is the index of the ID.
func (b *Builder) ColumnIndexes() []int { return b.columnIndexes }

// FieldKinds returns the kinds of the stored fields by their indexes in the struct, or nil when all of them are
// FieldDefault.
func (b *Builder) FieldKinds() []FieldKind { return b.fieldKinds }

func (b *Builder) UpdateIndexes() []int { return b.updateIndexes }

func (b *Builder) TimeIndexes() []int { return b.timeIndexes }

func (b *Builder) PasswordIndexes() []int { return b.passwordIndexes }

// UpdateFieldNames returns an ordered list of names of the fields that are written when an object is updated.
// It is the order of columns in the SET clause of the generated UPDATE BY ID and INSERT ON CONFLICT UPDATE queries.
func (b *Builder) UpdateFieldNames() []string { return b.structInfo.UpdateFieldNames }

func (b *Builder) VersionField() string { return b.structInfo.VersionField }

// IsNullable returns whether the column of the field can be NULL.
func (b *Builder) IsNullable(field string) bool {
	info, ok := b.structInfo.Fields[field]
	return ok && info.Nullable
}

func (b *Builder) TimeFields() []string { return b.structInfo.TimeFields }

func (b *Builder) JSONFields() []string { return b.structInfo.JSONFields }

// ValuerType returns the Valuer type of a stored field, or of what it points to, or nil when it is not of one.
func (b *Builder) ValuerType(field string) reflect.Type { return b.valuerTypes[field] }

func (b *Builder) StructName() string { return b.structInfo.Name }

// TableName returns the name of the table, without quotes.
func (b *Builder) TableName() string { return b.tableName }

// Columns describes the columns of the table, in the order of the struct fields.
func (b *Builder) Columns() []Column { return b.columns }

// IndexStatements returns a CREATE INDEX IF NOT EXISTS statement for each index declared with 'idx' or 'uidx' tags.
func (b *Builder) IndexStatements() []string { return b.indexStatements }

// IndexNames returns the names of the indexes created by IndexStatements.
func (b *Builder) IndexNames() []string { return b.indexNames }

// Relations returns the fields that hold related objects, declared with 'rel' tags.
func (b *Builder) Relations() []*structinfo.RelationInfo { return b.structInfo.Relations }

func (b *Builder) UniqueFields() []string { return b.structInfo.UniqueFields }

// UniqueKeyFields returns the fields of the unique column or index that the errors of the database name with key, see
// dialect.Dialect.UniqueKey, or nil when it is not one of the struct.
func (b *Builder) UniqueKeyFields(key string) []string { return b.uniqueKeyFields[key] }

func (b *Builder) PasswordFields() []string { return b.structInfo.PasswordFields }

// Select returns a SELECT query with a WHERE condition built from 'filters'.
func (b *Builder) Select(order []string, limit int, offset int, filters sqlfilters.Filter) (string, error) {
	query, err := b.selectQuery(order, limit, offset, filters)
	if err != nil {
		return "", getQueryBuilderError("Select", err)
	}
	return query + ";", nil
}

// SelectCount returns a SELECT COUNT(*) query to count rows with a WHERE condition built from 'filters'.
func (b *Builder) SelectCount(filters sqlfilters.Filter) (string, error) {
	query, err := b.selectCountQuery(filters)
	if err != nil {
		return "", getQueryBuilderError("SelectCount", err)
	}
	return query + ";", nil
}

// Delete returns a DELETE query with a WHERE condition built from 'filters'.
func (b *Builder) Delete(filters sqlfilters.Filter) (string, error) {
	query, err := b.deleteQuery(filters)
	if err != nil {
		return "", getQueryBuilderError("Delete", err)
	}
	return query + ";", nil
}

// DeleteReturningID returns a DELETE query with a WHERE condition built from 'filters' with RETURNING id.
func (b *Builder) DeleteReturningID(filters sqlfilters.Filter) (string, error) {
	query, err := b.deleteReturningIDQuery(filters)
	if err != nil {
		return "", getQueryBuilderError("DeleteReturningID", err)
	}
	return query + ";", nil
}

// Update returns an UPDATE query of the fields in values, of the rows that match filters. The values of the
// placeholders are values sorted by field name, followed by the values of filters.
func (b *Builder) Update(values map[string]any, filters sqlfilters.Filter) (string, error) {
	query, err := b.updateQuery(values, filters)
	if err != nil {
		return "", getQueryBuilderError("Update", err)
	}
	return query + ";", nil
}
