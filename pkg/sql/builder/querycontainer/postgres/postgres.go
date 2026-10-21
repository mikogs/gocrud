package postgres

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mikogs/gocrud/pkg/sql/builder/querycontainer"
	"github.com/mikogs/gocrud/pkg/structinfo"
)

type QueryContainer struct {
	TableName         string
	ColumnFieldName   map[string]string
	ColumnDefinitions []string
	ColumnNames       []string
	// UpdateColumnNames contains the quoted names of the columns that are written when an object is updated,
	// see structinfo.StructInfo.UpdateFieldNames.
	UpdateColumnNames []string
	ColumnNotString   map[string]bool

	FieldColumnName map[string]string

	// RawTableName is the name of the table, without quotes.
	RawTableName string
	// Columns describes the columns of the table, in the same order as ColumnNames.
	Columns []querycontainer.Column
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

func New(obj interface{}, structInfo *structinfo.StructInfo, tableNamePrefix string) (*QueryContainer, error) {
	queryContainer := &QueryContainer{}

	objType := reflect.TypeOf(obj).Elem()

	objTypeName := objType.Name()
	// if a struct is User_Register, then take User as base for table name.
	if strings.Contains(objTypeName, "_") {
		objTypeName = strings.Split(objTypeName, "_")[0]
	}

	rawTableName := tableNamePrefix + FieldToColumn(objTypeName)
	queryContainer.TableName = fmt.Sprintf(`"%s"`, rawTableName)
	queryContainer.RawTableName = rawTableName

	numField := objType.NumField()

	queryContainer.ColumnFieldName = make(map[string]string, numField)
	queryContainer.ColumnDefinitions = make([]string, 0, numField)
	queryContainer.ColumnNames = make([]string, 0, numField)
	queryContainer.ColumnNotString = make(map[string]bool, numField)
	queryContainer.FieldColumnName = make(map[string]string, numField)

	for j := 0; j < numField; j++ {
		field := objType.Field(j)

		// Only basic golang types are included as columns for the database table.
		// Check the function below for the details.
		if !field.IsExported() || !structinfo.IsFieldTypeSupported(field.Type) {
			continue
		}

		// Skip ignored fields (tagged with "-")
		if structInfo.Fields[field.Name].Ignored {
			continue
		}

		columnName := FieldToColumn(field.Name)
		if structInfo.Fields[field.Name].OverrideColumnName != "" {
			columnName = structInfo.Fields[field.Name].OverrideColumnName
		}

		queryContainer.FieldColumnName[field.Name] = columnName

		if structinfo.BaseType(field.Type).Kind() != reflect.String {
			queryContainer.ColumnNotString[columnName] = true
		}

		queryContainer.ColumnFieldName[columnName] = field.Name

		unique := false
		if structInfo.Fields[field.Name].Unique {
			unique = true
		}

		columnDefinition := columnDefinitionFromField(field.Name, structinfo.BaseType(field.Type).String(), unique, structInfo.Fields[field.Name].OverrideColumnType, structInfo.Fields[field.Name].Nullable)
		queryContainer.ColumnDefinitions = append(queryContainer.ColumnDefinitions, fmt.Sprintf(`%s %s`, QuoteColumn(ColumnWithoutAlias(columnName)), columnDefinition))
		queryContainer.Columns = append(queryContainer.Columns, querycontainer.Column{
			Name:     ColumnWithoutAlias(columnName),
			Type:     columnType(structinfo.BaseType(field.Type).String(), structInfo.Fields[field.Name].OverrideColumnType),
			Nullable: structInfo.Fields[field.Name].Nullable,
			Unique:   unique,
			IsID:     field.Name == "ID",
			AddStatements: []string{
				fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, queryContainer.TableName, QuoteColumn(ColumnWithoutAlias(columnName)), columnDefinition),
			},
		})
		queryContainer.ColumnNames = append(queryContainer.ColumnNames, QuoteColumn(ColumnWithoutAlias(columnName)))
	}

	for _, fieldName := range structInfo.UpdateFieldNames {
		queryContainer.UpdateColumnNames = append(queryContainer.UpdateColumnNames, QuoteColumn(ColumnWithoutAlias(queryContainer.FieldColumnName[fieldName])))
	}

	queryContainer.IndexStatements, queryContainer.IndexNames = indexStatements(rawTableName, queryContainer.TableName, structInfo.Indexes, queryContainer.FieldColumnName)

	// structinfo.New ensures that ID is the first column
	err := queryContainer.setQueries(obj, tableNamePrefix)
	if err != nil {
		return nil, err
	}

	return queryContainer, nil
}

func (q *QueryContainer) setQueries(obj interface{}, tableNamePrefix string) (err error) {
	numColumn := len(q.ColumnNames)
	numUpdateColumn := len(q.UpdateColumnNames)

	columnNames := strings.Join(q.ColumnNames, ",")
	columnNamesWithoutID := strings.Join(q.ColumnNames[1:], ",")
	valuesWithoutID := placeholders(1, numColumn-1)
	values := placeholders(1, numColumn)

	// UPDATE BY ID takes the values of the updated columns followed by the ID, and INSERT ON CONFLICT UPDATE
	// takes the values of all the columns followed by the values of the updated columns
	updateSet := setClause(q.UpdateColumnNames, 1)
	onConflictUpdateSet := setClause(q.UpdateColumnNames, numColumn+1)

	idColumn := `"id"`

	if dropTableBuilderImpl, ok := obj.(querycontainer.DropTableBuilder); ok {
		query, err := dropTableBuilderImpl.BuildDropTableQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.DropTable = query
	} else {
		q.DropTable = fmt.Sprintf("DROP TABLE IF EXISTS %s", q.TableName)
	}

	if createTableBuilderImpl, ok := obj.(querycontainer.CreateTableBuilder); ok {
		query, err := createTableBuilderImpl.BuildCreateTableQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.CreateTable = query
	} else {
		statements := append([]string{fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", q.TableName, strings.Join(q.ColumnDefinitions, ","))}, q.IndexStatements...)
		q.CreateTable = strings.Join(statements, "; ")
	}

	if deleteByIDBuilderImpl, ok := obj.(querycontainer.DeleteByIDBuilder); ok {
		query, err := deleteByIDBuilderImpl.BuildDeleteByIDQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.DeleteByID = query
	} else {
		q.DeleteByID = fmt.Sprintf("DELETE FROM %s WHERE %s = $1", q.TableName, idColumn)
	}

	if deletePrefixBuilderImpl, ok := obj.(querycontainer.DeletePrefixBuilder); ok {
		query, err := deletePrefixBuilderImpl.BuildDeletePrefixQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.DeletePrefix = query
	} else {
		q.DeletePrefix = fmt.Sprintf("DELETE FROM %s", q.TableName)
	}

	if updateByIDBuilderImpl, ok := obj.(querycontainer.UpdateByIDBuilder); ok {
		query, err := updateByIDBuilderImpl.BuildUpdateByIDQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.UpdateByID = query
	} else {
		q.UpdateByID = fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d", q.TableName, updateSet, idColumn, numUpdateColumn+1)
	}

	if updatePrefixBuilderImpl, ok := obj.(querycontainer.UpdatePrefixBuilder); ok {
		query, err := updatePrefixBuilderImpl.BuildUpdatePrefixQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.UpdatePrefix = query
	} else {
		q.UpdatePrefix = fmt.Sprintf("UPDATE %s SET", q.TableName)
	}

	if insertBuilderImpl, ok := obj.(querycontainer.InsertBuilder); ok {
		query, err := insertBuilderImpl.BuildInsertQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.Insert = query
	} else {
		q.Insert = fmt.Sprintf("INSERT INTO %s(%s) VALUES (%s) RETURNING %s", q.TableName, columnNamesWithoutID, valuesWithoutID, idColumn)
	}

	if insertOnConflictUpdateBuilderImpl, ok := obj.(querycontainer.InsertOnConflictUpdateBuilder); ok {
		query, err := insertOnConflictUpdateBuilderImpl.BuildInsertOnConflictUpdateQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.InsertOnConflictUpdate = query
	} else {
		q.InsertOnConflictUpdate = fmt.Sprintf("INSERT INTO %s(%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s RETURNING %s", q.TableName, columnNames, values, idColumn, onConflictUpdateSet, idColumn)
	}

	if selectByIDBuilderImpl, ok := obj.(querycontainer.SelectByIDBuilder); ok {
		query, err := selectByIDBuilderImpl.BuildSelectByIDQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.SelectByID = query
	} else {
		q.SelectByID = fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1", columnNames, q.TableName, idColumn)
	}

	if selectPrefixBuilderImpl, ok := obj.(querycontainer.SelectPrefixBuilder); ok {
		query, err := selectPrefixBuilderImpl.BuildSelectPrefixQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.SelectPrefix = query
		q.CustomSelectPrefix = true
	} else {
		q.SelectPrefix = fmt.Sprintf("SELECT %s FROM %s", columnNames, q.TableName)
	}

	if selectCountPrefixBuilderImpl, ok := obj.(querycontainer.SelectCountPrefixBuilder); ok {
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
