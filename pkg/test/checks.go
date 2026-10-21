package test

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"

	structInfo "github.com/mikogs/gocrud/pkg/structinfo"
)

const IDField = "ID"

func ObjFieldInterfaces(obj interface{}, includeID bool) []interface{} {
	objValue := reflect.ValueOf(obj).Elem()

	var fieldInterfaces []interface{}

	for i := 0; i < objValue.NumField(); i++ {
		valueField := objValue.Field(i)
		if objValue.Type().Field(i).Name == IDField && !includeID {
			continue
		}

		// builder is used to generate SQL queries, so here the same kinds must be supported
		if !objValue.Type().Field(i).IsExported() || !structInfo.IsFieldKindSupported(valueField.Kind()) {
			continue
		}

		fieldInterfaces = append(fieldInterfaces, valueField.Addr().Interface())
	}

	return fieldInterfaces
}

func FatalIfTestStructNotDeletedInTheDatabase(t *testing.T, db *sql.DB, id uint64) {
	var rowCount uint64
	err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) AS c FROM test_struct WHERE id = %d", id)).Scan(&rowCount)
	if err != nil {
		t.Fatalf("Delete failed to delete struct from the table")
	}
	if rowCount > 0 {
		t.Fatalf("Delete failed to delete struct from the table")
	}
}

func FatalIfNoTestStructInTheDatabase(t *testing.T, db *sql.DB) *TestStruct {
	objLoaded := &TestStruct{}
	fieldsPtrs := []interface{}{&objLoaded.ID}
	fieldsPtrs = append(fieldsPtrs, ObjFieldInterfaces(objLoaded, false)...)
	err := db.QueryRow("SELECT * FROM test_struct ORDER BY id DESC LIMIT 1").Scan(fieldsPtrs...)
	if err != nil {
		t.Fatalf("Save failed to insert struct to the table: %s", err.Error())
	}

	return objLoaded
}
