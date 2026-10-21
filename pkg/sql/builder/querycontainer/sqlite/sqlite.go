package sqlite

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
	qc := &QueryContainer{}

	objType := reflect.TypeOf(obj).Elem()

	objTypeName := objType.Name()
	if strings.Contains(objTypeName, "_") {
		objTypeName = strings.Split(objTypeName, "_")[0]
	}

	rawTableName := tableNamePrefix + FieldToColumn(objTypeName)
	qc.TableName = fmt.Sprintf(`"%s"`, rawTableName)
	qc.RawTableName = rawTableName

	numField := objType.NumField()
	qc.ColumnFieldName = make(map[string]string, numField)
	qc.ColumnDefinitions = make([]string, 0, numField)
	qc.ColumnNames = make([]string, 0, numField)
	qc.ColumnNotString = make(map[string]bool, numField)
	qc.FieldColumnName = make(map[string]string, numField)

	for j := 0; j < numField; j++ {
		field := objType.Field(j)

		if !field.IsExported() || !structinfo.IsFieldTypeSupported(field.Type) {
			continue
		}

		if structInfo.Fields[field.Name].Ignored {
			continue
		}

		columnName := FieldToColumn(field.Name)
		if structInfo.Fields[field.Name].OverrideColumnName != "" {
			columnName = structInfo.Fields[field.Name].OverrideColumnName
		}

		qc.FieldColumnName[field.Name] = columnName

		if structinfo.BaseType(field.Type).Kind() != reflect.String {
			qc.ColumnNotString[columnName] = true
		}

		qc.ColumnFieldName[columnName] = field.Name

		unique := structInfo.Fields[field.Name].Unique
		foreignKey := querycontainer.NewForeignKey(tableNamePrefix, structInfo.Fields[field.Name].ForeignKeyTable, structInfo.Fields[field.Name].OnDelete)
		columnDefinition := columnDefinitionFromField(field.Name, structinfo.BaseType(field.Type).String(), unique, structInfo.Fields[field.Name].OverrideColumnType, structInfo.Fields[field.Name].Nullable) + foreignKey.Clause()
		qc.ColumnDefinitions = append(qc.ColumnDefinitions, fmt.Sprintf(`%s %s`, QuoteColumn(ColumnWithoutAlias(columnName)), columnDefinition))
		qc.Columns = append(qc.Columns, querycontainer.Column{
			Name:          ColumnWithoutAlias(columnName),
			Type:          columnType(structinfo.BaseType(field.Type).String(), structInfo.Fields[field.Name].OverrideColumnType),
			Nullable:      structInfo.Fields[field.Name].Nullable,
			Unique:        unique,
			IsID:          field.Name == "ID",
			ForeignKey:    foreignKey,
			AddStatements: addColumnStatements(rawTableName, ColumnWithoutAlias(columnName), field.Name, structinfo.BaseType(field.Type).String(), unique, structInfo.Fields[field.Name].OverrideColumnType, structInfo.Fields[field.Name].Nullable, foreignKey),
		})
		qc.ColumnNames = append(qc.ColumnNames, QuoteColumn(ColumnWithoutAlias(columnName)))
	}

	for _, fieldName := range structInfo.UpdateFieldNames {
		qc.UpdateColumnNames = append(qc.UpdateColumnNames, QuoteColumn(ColumnWithoutAlias(qc.FieldColumnName[fieldName])))
	}

	qc.IndexStatements, qc.IndexNames = indexStatements(rawTableName, qc.TableName, structInfo.Indexes, qc.FieldColumnName)

	// structinfo.New ensures that ID is the first column
	err := qc.setQueries(obj, tableNamePrefix)
	if err != nil {
		return nil, err
	}

	return qc, nil
}

func (q *QueryContainer) setQueries(obj interface{}, tableNamePrefix string) error {
	numColumn := len(q.ColumnNames)

	columnNames := strings.Join(q.ColumnNames, ",")
	columnNamesWithoutID := strings.Join(q.ColumnNames[1:], ",")

	// SQLite uses ? for all placeholders
	placeholders := func(n int) string {
		if n <= 0 {
			return ""
		}
		return strings.Repeat("?,", n)[:n*2-1]
	}

	valuesWithoutID := placeholders(numColumn - 1)
	values := placeholders(numColumn)

	// SET clause: "col1"=?,"col2"=? (updated columns)
	setCols := make([]string, len(q.UpdateColumnNames))
	for i, col := range q.UpdateColumnNames {
		setCols[i] = col + "=?"
	}
	setClause := strings.Join(setCols, ",")

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
		q.DeleteByID = fmt.Sprintf("DELETE FROM %s WHERE %s = ?", q.TableName, idColumn)
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
		q.UpdateByID = fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", q.TableName, setClause, idColumn)
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
		q.InsertOnConflictUpdate = fmt.Sprintf("INSERT INTO %s(%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s RETURNING %s", q.TableName, columnNames, values, idColumn, setClause, idColumn)
	}

	if selectByIDBuilderImpl, ok := obj.(querycontainer.SelectByIDBuilder); ok {
		query, err := selectByIDBuilderImpl.BuildSelectByIDQuery(tableNamePrefix)
		if err != nil {
			return err
		}
		q.SelectByID = query
	} else {
		q.SelectByID = fmt.Sprintf("SELECT %s FROM %s WHERE %s = ?", columnNames, q.TableName, idColumn)
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
