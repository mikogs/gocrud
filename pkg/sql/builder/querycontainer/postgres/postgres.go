package postgres

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mikolajgasior/gocrud/pkg/sql/builder/querycontainer"
	"github.com/mikolajgasior/gocrud/pkg/structinfo"
)

type QueryContainer struct {
	TableName         string
	ColumnFieldName   map[string]string
	ColumnDefinitions []string
	ColumnNames       []string
	ColumnNotString   map[string]bool

	FieldColumnName map[string]string

	// IndexStatements contains a CREATE INDEX statement for each index declared via
	// 'idx'/'uidx' struct tags. It is empty when the struct has no indexed fields.
	IndexStatements []string

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

func New(obj interface{}, structInfo *structinfo.StructInfo, tableNamePrefix string) *QueryContainer {
	queryContainer := &QueryContainer{}

	objValue := reflect.ValueOf(obj)
	objIndirectValue := reflect.Indirect(objValue)
	objType := objIndirectValue.Type()

	if objType.String() == "reflect.Value" {
		objType = reflect.ValueOf(obj.(reflect.Value).Interface()).Type().Elem().Elem()
	}

	objTypeName := objType.Name()
	// if a struct is User_Register, then take User as base for table name.
	if strings.Contains(objTypeName, "_") {
		objTypeName = strings.Split(objTypeName, "_")[0]
	}

	rawTableName := tableNamePrefix + FieldToColumn(objTypeName)
	queryContainer.TableName = fmt.Sprintf(`"%s"`, rawTableName)

	numField := objType.NumField()

	queryContainer.ColumnFieldName = make(map[string]string, numField)
	queryContainer.ColumnDefinitions = make([]string, 0, numField)
	queryContainer.ColumnNames = make([]string, 0, numField)
	queryContainer.ColumnNotString = make(map[string]bool, numField)
	queryContainer.FieldColumnName = make(map[string]string, numField)

	for j := 0; j < numField; j++ {
		field := objType.Field(j)
		fieldTypeKind := field.Type.Kind()

		// Only basic golang types are included as columns for the database table.
		// Check the function below for the details.
		if !structinfo.IsFieldKindSupported(fieldTypeKind) {
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

		if fieldTypeKind != reflect.String {
			queryContainer.ColumnNotString[columnName] = true
		}

		queryContainer.ColumnFieldName[columnName] = field.Name

		unique := false
		if structInfo.Fields[field.Name].Unique {
			unique = true
		}

		columnDefinition := columnDefinitionFromField(field.Name, field.Type.String(), unique, structInfo.Fields[field.Name].OverrideColumnType)
		queryContainer.ColumnDefinitions = append(queryContainer.ColumnDefinitions, fmt.Sprintf(`%s %s`, QuoteColumn(columnName), columnDefinition))
		queryContainer.ColumnNames = append(queryContainer.ColumnNames, QuoteColumn(columnName))
	}

	queryContainer.IndexStatements = indexStatements(rawTableName, queryContainer.TableName, structInfo.Indexes, queryContainer.FieldColumnName)

	// Assuming that primary field is named ID and that it is always first -> TODO: add check
	err := queryContainer.setQueries(obj, tableNamePrefix)
	if err != nil {
		panic(err)
	}

	return queryContainer
}

func (q *QueryContainer) setQueries(obj interface{}, tableNamePrefix string) (err error) {
	var (
		columnNames                string
		columnNamesWithoutID       string
		valuesWithoutID            string
		values                     string
		columnNamesWithValues      string
		columnNamesWithValuesAgain string
		numColumn                  int
	)

	// Assuming that struct has at least 2 fields -> TODO: add check
	numColumn = len(q.ColumnNames)

	columnNamesWithoutID = strings.Join(q.ColumnNames[1:], ",")
	columnNames = strings.Join(q.ColumnNames, ",")
	valuesWithoutID = "?" + strings.Repeat(",?", numColumn-2)

	columnNamesWithValues = strings.Join(q.ColumnNames[1:], "=?,") + "=?"
	columnNamesWithValuesAgain = columnNamesWithValues
	for i := 1; i <= numColumn*2; i++ {
		columnNamesWithValues = strings.Replace(columnNamesWithValues, "?", fmt.Sprintf("$%d", i), 1)
		valuesWithoutID = strings.Replace(valuesWithoutID, "?", fmt.Sprintf("$%d", i), 1)
		if i > numColumn {
			columnNamesWithValuesAgain = strings.Replace(columnNamesWithValuesAgain, "?", fmt.Sprintf("$%d", i), 1)
		}
	}
	values = valuesWithoutID + fmt.Sprintf(",$%d", numColumn)

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
		q.CreateTable = query
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
		q.UpdateByID = fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d", q.TableName, columnNamesWithValues, idColumn, numColumn)
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
		q.InsertOnConflictUpdate = fmt.Sprintf("INSERT INTO %s(%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s RETURNING %s", q.TableName, columnNames, values, idColumn, columnNamesWithValuesAgain, idColumn)
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
	} else {
		q.SelectPrefix = fmt.Sprintf("SELECT %s FROM %s", columnNames, q.TableName)
	}

	if selectCountPrefixBuilderImpl, ok := obj.(querycontainer.SelectCountPrefixBuilder); ok {
		query, err := selectCountPrefixBuilderImpl.BuildSelectCountPrefixQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.SelectCountPrefix = query
	} else {
		q.SelectCountPrefix = fmt.Sprintf("SELECT COUNT(*) AS cnt FROM %s", q.TableName)
	}

	return nil
}
