package sqlbuilder

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mikogs/gocrud/internal/dialect"
	"github.com/mikogs/gocrud/internal/structinfo"
)

// The interfaces of the structs that build their own statements, with the table name prefix.

type DropTableBuilder interface {
	BuildDropTableQuery(tableNamePrefix string) (string, error)
}

type CreateTableBuilder interface {
	BuildCreateTableQuery(tableNamePrefix string) (string, error)
}

type DeleteByIDBuilder interface {
	BuildDeleteByIDQuery(tableNamePrefix string) (string, error)
}

type DeletePrefixBuilder interface {
	BuildDeletePrefixQuery(tableNamePrefix string) (string, error)
}

type UpdateByIDBuilder interface {
	BuildUpdateByIDQuery(tableNamePrefix string) (string, error)
}

type UpdatePrefixBuilder interface {
	BuildUpdatePrefixQuery(tableNamePrefix string) (string, error)
}

type InsertBuilder interface {
	BuildInsertQuery(tableNamePrefix string) (string, error)
}

type InsertOnConflictUpdateBuilder interface {
	BuildInsertOnConflictUpdateQuery(tableNamePrefix string) (string, error)
}

type SelectByIDBuilder interface {
	BuildSelectByIDQuery(tableNamePrefix string) (string, error)
}

type SelectPrefixBuilder interface {
	BuildSelectPrefixQuery(tableNamePrefix string) (string, error)
}

type SelectCountPrefixBuilder interface {
	BuildSelectCountPrefixQuery(tableNamePrefix string) (string, error)
}

// statements are the statements of a struct that do not depend on filters, e.g. CREATE TABLE and INSERT, and the
// names of its table and columns.
type statements struct {
	TableName         string
	ColumnFieldName   map[string]string
	ColumnDefinitions []string
	ColumnNames       []string
	// UpdateColumnNames contains the quoted names of the columns that are written when an object is updated,
	// see structinfo.StructInfo.UpdateFieldNames.
	UpdateColumnNames []string
	// PasswordColumnNames contains the quoted names of the columns of the password fields. An update keeps the stored
	// password when the value of such a column is empty.
	PasswordColumnNames map[string]bool
	// ColumnNotString contains the columns whose values are not text in the database, e.g. numbers, and UUIDs when the
	// database has a type of UUIDs.
	ColumnNotString map[string]bool

	FieldColumnName map[string]string

	// RawTableName is the name of the table, without quotes.
	RawTableName string
	// Columns describes the columns of the table, in the same order as ColumnNames.
	Columns []Column
	// IndexNames contains the names of the indexes created by IndexStatements.
	IndexNames []string

	// IndexStatements contains a CREATE INDEX statement for each index declared via
	// 'idx'/'uidx' struct tags. It is empty when the struct has no indexed fields.
	IndexStatements []string

	// CustomSelectPrefix and CustomSelectCountPrefix are set to true when the object builds its own
	// SELECT prefixes. Only then column aliases (eg. "t1.name") are used, as only the custom FROM clause defines them.
	CustomSelectPrefix      bool
	CustomSelectCountPrefix bool

	CreateTable string
	DropTable   string

	Insert                 string
	InsertOnConflictUpdate string

	UpdateByID string
	SelectByID string
	DeleteByID string

	SelectPrefix      string
	SelectCountPrefix string
	DeletePrefix      string
	UpdatePrefix      string
}

func newStatements(obj interface{}, structInfo *structinfo.StructInfo, tableNamePrefix string, d dialect.Dialect) (*statements, error) {
	q := &statements{}

	objType := reflect.TypeOf(obj).Elem()

	objTypeName := objType.Name()
	// if a struct is User_Register, then take User as base for table name.
	if strings.Contains(objTypeName, "_") {
		objTypeName = strings.Split(objTypeName, "_")[0]
	}

	rawTableName := tableNamePrefix + fieldToColumn(objTypeName)
	q.TableName = fmt.Sprintf(`"%s"`, rawTableName)
	q.RawTableName = rawTableName

	numField := objType.NumField()

	q.ColumnFieldName = make(map[string]string, numField)
	q.ColumnDefinitions = make([]string, 0, numField)
	q.ColumnNames = make([]string, 0, numField)
	q.ColumnNotString = make(map[string]bool, numField)
	q.FieldColumnName = make(map[string]string, numField)

	for j := 0; j < numField; j++ {
		field := objType.Field(j)

		if !field.IsExported() || !structinfo.IsFieldTypeSupported(field.Type) {
			continue
		}

		fieldInfo := structInfo.Fields[field.Name]
		if fieldInfo.Ignored {
			continue
		}

		columnName := fieldToColumn(field.Name)
		if fieldInfo.OverrideColumnName != "" {
			columnName = fieldInfo.OverrideColumnName
		}

		q.FieldColumnName[field.Name] = columnName

		// a UUID is not a string in a database with a type of UUIDs, e.g. LIKE needs it cast to text
		isUUID := (fieldInfo.UUID || (field.Name == "ID" && structInfo.UUID)) && d.UUIDType() != ""
		fieldType := structinfo.BaseType(field.Type).String()
		if structinfo.BaseType(field.Type).Kind() != reflect.String || isUUID {
			q.ColumnNotString[columnName] = true
		}

		q.ColumnFieldName[columnName] = field.Name

		foreignKey := NewForeignKey(tableNamePrefix, fieldInfo.ForeignKeyTable, fieldInfo.OnDelete)
		definition := func(unique bool) string {
			return columnDefinition(d, field.Name, fieldType, unique, fieldInfo.OverrideColumnType, fieldInfo.Nullable, fieldInfo.UUID) + foreignKey.Clause()
		}
		q.ColumnDefinitions = append(q.ColumnDefinitions, fmt.Sprintf(`%s %s`, quoteName(columnWithoutAlias(columnName)), definition(fieldInfo.Unique)))

		columnType := columnType(d, fieldType, fieldInfo.OverrideColumnType)
		if isUUID {
			columnType = d.UUIDType()
		}
		q.Columns = append(q.Columns, Column{
			Name:          columnWithoutAlias(columnName),
			Type:          columnType,
			Nullable:      fieldInfo.Nullable,
			Unique:        fieldInfo.Unique,
			IsID:          field.Name == "ID",
			ForeignKey:    foreignKey,
			AddStatements: d.AddColumn(rawTableName, columnWithoutAlias(columnName), definition, fieldInfo.Unique),
		})
		q.ColumnNames = append(q.ColumnNames, quoteName(columnWithoutAlias(columnName)))
	}

	for _, fieldName := range structInfo.UpdateFieldNames {
		q.UpdateColumnNames = append(q.UpdateColumnNames, quoteName(columnWithoutAlias(q.FieldColumnName[fieldName])))
	}

	q.PasswordColumnNames = make(map[string]bool, len(structInfo.PasswordFields))
	for _, fieldName := range structInfo.PasswordFields {
		q.PasswordColumnNames[quoteName(columnWithoutAlias(q.FieldColumnName[fieldName]))] = true
	}

	q.IndexStatements, q.IndexNames = indexStatements(rawTableName, q.TableName, structInfo.Indexes, q.FieldColumnName)

	// structinfo.New ensures that ID is the first column
	err := q.setQueries(obj, tableNamePrefix, structInfo.UUID, d)
	if err != nil {
		return nil, err
	}

	return q, nil
}

// columnDefinition returns the definition of the column of a field. Nullable columns do not have NOT NULL and
// DEFAULT, so that their values can be NULL. A foreign key with a UUID, in a database with a type of UUIDs, has no
// default, as an empty string is not a UUID.
func columnDefinition(d dialect.Dialect, fieldName string, fieldType string, isUnique bool, overrideColumnType string, isNullable bool, isUUID bool) string {
	// 'ID' is a special field, a string ID holds a UUID
	if fieldName == "ID" {
		return d.IDColumn(fieldType == "string")
	}

	columnType, defaultValue := d.ColumnType(fieldType)
	if overrideColumnType != "" {
		columnType = overrideColumnType
		defaultValue = "''"
	}

	definition := columnType
	switch {
	case isUUID && d.UUIDType() != "":
		definition = d.UUIDType()
		if !isNullable {
			definition += " NOT NULL"
		}
	case !isNullable:
		definition += " NOT NULL DEFAULT " + defaultValue
	}

	if isUnique {
		definition += " UNIQUE"
	}

	return definition
}

// columnType returns the type of the column for a field type, or the type set in the field tag.
func columnType(d dialect.Dialect, fieldType string, overrideColumnType string) string {
	if overrideColumnType != "" {
		return overrideColumnType
	}

	columnType, _ := d.ColumnType(fieldType)
	return columnType
}

// placeholders returns num comma-separated placeholders, of the values from first.
func placeholders(d dialect.Dialect, first int, num int) string {
	values := make([]string, num)
	for i := range values {
		values[i] = d.Placeholder(first + i)
	}

	return strings.Join(values, ",")
}

// setClause returns a SET clause for the quoted columns of table, with placeholders of the values from first, which
// keeps the stored passwords of the password columns when their values are empty.
func setClause(d dialect.Dialect, columns []string, first int, table string, passwordColumns map[string]bool) string {
	values := make([]string, len(columns))
	for i, column := range columns {
		values[i] = fmt.Sprintf("%s=%s", column, keepEmptyPassword(d.Placeholder(first+i), table, column, passwordColumns[column]))
	}

	return strings.Join(values, ",")
}

// setQueries sets the queries of the struct. The INSERT of a struct with a UUID includes the ID, which is generated
// before the object is inserted, instead of the one assigned by the database.
func (q *statements) setQueries(obj interface{}, tableNamePrefix string, uuid bool, d dialect.Dialect) (err error) {
	numColumn := len(q.ColumnNames)
	numUpdateColumn := len(q.UpdateColumnNames)

	columnNames := strings.Join(q.ColumnNames, ",")
	columnNamesWithoutID := strings.Join(q.ColumnNames[1:], ",")
	valuesWithoutID := placeholders(d, 1, numColumn-1)
	values := placeholders(d, 1, numColumn)

	// UPDATE BY ID takes the values of the updated columns followed by the ID, and INSERT ON CONFLICT UPDATE
	// takes the values of all the columns followed by the values of the updated columns
	updateSet := setClause(d, q.UpdateColumnNames, 1, q.TableName, q.PasswordColumnNames)
	onConflictUpdateSet := setClause(d, q.UpdateColumnNames, numColumn+1, q.TableName, q.PasswordColumnNames)

	idColumn := `"id"`

	if dropTableBuilderImpl, ok := obj.(DropTableBuilder); ok {
		query, err := dropTableBuilderImpl.BuildDropTableQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.DropTable = query
	} else {
		q.DropTable = fmt.Sprintf("DROP TABLE IF EXISTS %s", q.TableName)
	}

	if createTableBuilderImpl, ok := obj.(CreateTableBuilder); ok {
		query, err := createTableBuilderImpl.BuildCreateTableQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.CreateTable = query
	} else {
		statements := append([]string{fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", q.TableName, strings.Join(q.ColumnDefinitions, ","))}, q.IndexStatements...)
		q.CreateTable = strings.Join(statements, "; ")
	}

	if deleteByIDBuilderImpl, ok := obj.(DeleteByIDBuilder); ok {
		query, err := deleteByIDBuilderImpl.BuildDeleteByIDQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.DeleteByID = query
	} else {
		q.DeleteByID = fmt.Sprintf("DELETE FROM %s WHERE %s = %s", q.TableName, idColumn, d.Placeholder(1))
	}

	if deletePrefixBuilderImpl, ok := obj.(DeletePrefixBuilder); ok {
		query, err := deletePrefixBuilderImpl.BuildDeletePrefixQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.DeletePrefix = query
	} else {
		q.DeletePrefix = fmt.Sprintf("DELETE FROM %s", q.TableName)
	}

	if updateByIDBuilderImpl, ok := obj.(UpdateByIDBuilder); ok {
		query, err := updateByIDBuilderImpl.BuildUpdateByIDQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.UpdateByID = query
	} else {
		q.UpdateByID = fmt.Sprintf("UPDATE %s SET %s WHERE %s = %s", q.TableName, updateSet, idColumn, d.Placeholder(numUpdateColumn+1))
	}

	if updatePrefixBuilderImpl, ok := obj.(UpdatePrefixBuilder); ok {
		query, err := updatePrefixBuilderImpl.BuildUpdatePrefixQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.UpdatePrefix = query
	} else {
		q.UpdatePrefix = fmt.Sprintf("UPDATE %s SET", q.TableName)
	}

	if insertBuilderImpl, ok := obj.(InsertBuilder); ok {
		query, err := insertBuilderImpl.BuildInsertQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.Insert = query
	} else {
		q.Insert = fmt.Sprintf("INSERT INTO %s(%s) VALUES (%s) RETURNING %s", q.TableName, columnNamesWithoutID, valuesWithoutID, idColumn)
		if uuid {
			q.Insert = fmt.Sprintf("INSERT INTO %s(%s) VALUES (%s) RETURNING %s", q.TableName, columnNames, values, idColumn)
		}
	}

	if insertOnConflictUpdateBuilderImpl, ok := obj.(InsertOnConflictUpdateBuilder); ok {
		query, err := insertOnConflictUpdateBuilderImpl.BuildInsertOnConflictUpdateQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.InsertOnConflictUpdate = query
	} else {
		q.InsertOnConflictUpdate = fmt.Sprintf("INSERT INTO %s(%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s RETURNING %s", q.TableName, columnNames, values, idColumn, onConflictUpdateSet, idColumn)
	}

	if selectByIDBuilderImpl, ok := obj.(SelectByIDBuilder); ok {
		query, err := selectByIDBuilderImpl.BuildSelectByIDQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.SelectByID = query
	} else {
		q.SelectByID = fmt.Sprintf("SELECT %s FROM %s WHERE %s = %s", columnNames, q.TableName, idColumn, d.Placeholder(1))
	}

	if selectPrefixBuilderImpl, ok := obj.(SelectPrefixBuilder); ok {
		query, err := selectPrefixBuilderImpl.BuildSelectPrefixQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.SelectPrefix = query
		q.CustomSelectPrefix = true
	} else {
		q.SelectPrefix = fmt.Sprintf("SELECT %s FROM %s", columnNames, q.TableName)
	}

	if selectCountPrefixBuilderImpl, ok := obj.(SelectCountPrefixBuilder); ok {
		query, err := selectCountPrefixBuilderImpl.BuildSelectCountPrefixQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.SelectCountPrefix = query
		q.CustomSelectCountPrefix = true
	} else {
		q.SelectCountPrefix = fmt.Sprintf("SELECT COUNT(*) AS cnt FROM %s", q.TableName)
	}

	return nil
}
