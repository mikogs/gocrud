package gocrud

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/test"
)

// ---------- Save ----------

func TestSQLite_Save(t *testing.T) {
	recreateTestStructTableSQLite()

	objSaved := test.TestStructWithData()
	err := testCRUDSQLite.Save(context.Background(), objSaved, SaveOptions{})
	if err != nil {
		t.Fatalf("SQLite Save failed to insert: %s", err.Error())
	}
	if objSaved.ID == 0 {
		t.Fatalf("SQLite Save did not populate ID")
	}

	objLoaded := &test.TestStruct{}
	fieldsPtrs := []interface{}{&objLoaded.ID}
	fieldsPtrs = append(fieldsPtrs, ObjFieldInterfaces(objLoaded, false)...)
	err = testDBSQLite.QueryRow("SELECT * FROM test_struct ORDER BY id DESC LIMIT 1").Scan(fieldsPtrs...)
	if err != nil {
		t.Fatalf("SQLite Save failed to read back inserted row: %s", err.Error())
	}
	if objLoaded.ID == 0 || objLoaded.PrimaryEmail != objSaved.PrimaryEmail || objLoaded.Key != objSaved.Key {
		t.Fatalf("SQLite Save inserted incorrect data")
	}

	// update
	objSaved.PrimaryEmail = "updated@example.com"
	objSaved.Age = 50
	err = testCRUDSQLite.Save(context.Background(), objSaved, SaveOptions{})
	if err != nil {
		t.Fatalf("SQLite Save failed to update: %s", err.Error())
	}

	objLoaded2 := &test.TestStruct{}
	fieldsPtrs2 := []interface{}{&objLoaded2.ID}
	fieldsPtrs2 = append(fieldsPtrs2, ObjFieldInterfaces(objLoaded2, false)...)
	err = testDBSQLite.QueryRow("SELECT * FROM test_struct WHERE id=?", objSaved.ID).Scan(fieldsPtrs2...)
	if err != nil {
		t.Fatalf("SQLite Save failed to read back updated row: %s", err.Error())
	}
	if objLoaded2.PrimaryEmail != "updated@example.com" || objLoaded2.Age != 50 {
		t.Fatalf("SQLite Save did not persist the update")
	}
}

func TestSQLite_Save_InsertWithID(t *testing.T) {
	recreateTestStructTableSQLite()

	objSaved := test.TestStructWithData()
	objSaved.ID = 99999
	objSaved.FirstName = "ProvidedID"
	err := testCRUDSQLite.Save(context.Background(), objSaved, SaveOptions{})
	if err != nil {
		t.Fatalf("SQLite Save failed to insert with provided ID: %s", err.Error())
	}

	objLoaded := &test.TestStruct{}
	fieldsPtrs := ObjFieldInterfaces(objLoaded, true)
	err = testDBSQLite.QueryRow("SELECT * FROM test_struct WHERE id=99999").Scan(fieldsPtrs...)
	if err != nil {
		t.Fatalf("SQLite Save with provided ID not found: %s", err.Error())
	}
	if objLoaded.ID != 99999 || objLoaded.FirstName != "ProvidedID" {
		t.Fatalf("SQLite Save with provided ID stored wrong data")
	}

	objLoaded.FirstName = "UpdatedProvidedID"
	err = testCRUDSQLite.Save(context.Background(), objLoaded, SaveOptions{})
	if err != nil {
		t.Fatalf("SQLite Save failed to update struct with provided ID: %s", err.Error())
	}

	objLoaded2 := &test.TestStruct{}
	fieldsPtrs2 := ObjFieldInterfaces(objLoaded2, true)
	err = testDBSQLite.QueryRow("SELECT * FROM test_struct WHERE id=99999").Scan(fieldsPtrs2...)
	if err != nil {
		t.Fatalf("SQLite Save with provided ID update not found: %s", err.Error())
	}
	if objLoaded2.FirstName != "UpdatedProvidedID" {
		t.Fatalf("SQLite Save with provided ID did not update correctly")
	}
}

func TestSQLite_Save_InsertWithIDAndNoInsert(t *testing.T) {
	recreateTestStructTableSQLite()

	objSaved := test.TestStructWithData()
	objSaved.ID = 99999
	err := testCRUDSQLite.Save(context.Background(), objSaved, SaveOptions{NoInsert: true})
	if err != nil {
		t.Fatalf("SQLite Save with NoInsert returned unexpected error: %s", err.Error())
	}

	var rowCount int
	err = testDBSQLite.QueryRow("SELECT COUNT(*) FROM test_struct WHERE id=99999").Scan(&rowCount)
	if err != nil {
		t.Fatalf("SQLite count query failed: %s", err.Error())
	}
	if rowCount > 0 {
		t.Fatalf("SQLite Save with NoInsert should not have inserted a row")
	}
}

func TestSQLite_Save_ErrUniq(t *testing.T) {
	recreateTestStructTableSQLite()

	objSaved := test.TestStructWithData()
	objSaved.Key = "gottabeunique123456789012345678901234567890"
	_ = testCRUDSQLite.Save(context.Background(), objSaved, SaveOptions{})

	objWithSameKey := test.TestStructWithData()
	objWithSameKey.Key = "gottabeunique123456789012345678901234567890"
	err := testCRUDSQLite.Save(context.Background(), objWithSameKey, SaveOptions{})
	var uniqError *UniqError
	if !errors.As(err, &uniqError) || uniqError.Field != "test_struct.key" || uniqError.Error() != "uniq failed for field: test_struct.key" {
		t.Fatalf("SQLite Save did not return UniqError for test_struct.key on unique violation, got: %v", err)
	}
}

func TestSQLite_Save_InvalidObject(t *testing.T) {
	recreateTestStructTableSQLite()

	obj, wantViolations := invalidTestStruct()
	err := testCRUDSQLite.Save(context.Background(), obj, SaveOptions{})
	fatalIfNotViolations(t, err, wantViolations)

	cnt, _ := testCRUDSQLite.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{})
	if cnt != 0 {
		t.Fatalf("SQLite Save inserted an invalid object, got %d rows, want 0", cnt)
	}
}

// ---------- Load ----------

func TestSQLite_Load(t *testing.T) {
	recreateTestStructTableSQLite()

	objSaved := test.TestStructWithData()
	_ = testCRUDSQLite.Save(context.Background(), objSaved, SaveOptions{})

	objLoaded := &test.TestStruct{}
	output := testCRUDSQLite.Load(context.Background(), objLoaded, fmt.Sprintf("%d", objSaved.ID), LoadOptions{})
	if output.Error != nil {
		t.Fatalf("SQLite Load failed: %s", output.Error.Error())
	}
	if !test.AreTestStructObjectsSame(objSaved, objLoaded) {
		t.Fatalf("SQLite Load returned different data than what was saved")
	}
}

// ---------- Delete ----------

func TestSQLite_Delete(t *testing.T) {
	recreateTestStructTableSQLite()

	objSaved := test.TestStructWithData()
	_ = testCRUDSQLite.Save(context.Background(), objSaved, SaveOptions{})
	savedID := objSaved.ID

	err := testCRUDSQLite.Delete(context.Background(), objSaved, DeleteOptions{})
	if err != nil {
		t.Fatalf("SQLite Delete failed: %s", err.Error())
	}

	var rowCount int
	_ = testDBSQLite.QueryRow("SELECT COUNT(*) FROM test_struct WHERE id=?", savedID).Scan(&rowCount)
	if rowCount > 0 {
		t.Fatalf("SQLite Delete did not remove the row from the database")
	}
	if objSaved.ID != 0 {
		t.Fatalf("SQLite Delete did not zero the struct ID")
	}
}

// ---------- DeleteMultiple ----------

func TestSQLite_DeleteMultiple_WithFilters(t *testing.T) {
	recreateTestStructTableSQLite()

	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}

	_, err := testCRUDSQLite.DeleteMultiple(context.Background(), &test.TestStruct{}, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{
			"Price":        {Op: sqlfilters.OpEqual, Val: 444},
			"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "primary@example.com"},
		},
	})
	if err != nil {
		t.Fatalf("SQLite DeleteMultiple failed: %s", err.Error())
	}

	cnt, _ := testCRUDSQLite.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{})
	if cnt != 50 {
		t.Fatalf("SQLite DeleteMultiple removed wrong number of rows: got %d remaining, want 50", cnt)
	}
}

func TestSQLite_DeleteMultiple_WithRawQuery(t *testing.T) {
	recreateTestStructTableSQLite()

	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}

	_, err := testCRUDSQLite.DeleteMultiple(context.Background(), &test.TestStruct{}, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{
			"Price":        {Op: sqlfilters.OpEqual, Val: 444},
			"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "primary@example.com"},
			sqlfilters.RawKey: {
				Op: sqlfilters.OpOR,
				Val: []interface{}{
					".Age = ? OR .Age IN (?) OR (.Age = ? AND .PrimaryEmail = ?)",
					31,
					[]int{32, 33, 34},
					35,
					"miko@example.com",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SQLite DeleteMultiple with raw query failed: %s", err.Error())
	}

	cnt, _ := testCRUDSQLite.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{})
	if cnt != 46 {
		t.Fatalf("SQLite DeleteMultiple with raw query removed wrong number of rows: got %d remaining, want 46", cnt)
	}
}

func TestSQLite_DeleteMultiple_WithRawQueryOnly(t *testing.T) {
	recreateTestStructTableSQLite()

	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}

	_, err := testCRUDSQLite.DeleteMultiple(context.Background(), &test.TestStruct{}, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{
			sqlfilters.RawKey: {
				Op: sqlfilters.OpOR,
				Val: []interface{}{
					"(.Price = ? AND .PrimaryEmail = ?) OR (.Age = ? OR .Age IN (?) OR (.Age = ? AND .PrimaryEmail = ?))",
					444,
					"primary@example.com",
					31,
					[]int{32, 33, 34},
					35,
					"miko@example.com",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SQLite DeleteMultiple with raw-query-only failed: %s", err.Error())
	}

	cnt, _ := testCRUDSQLite.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{})
	if cnt != 46 {
		t.Fatalf("SQLite DeleteMultiple with raw-query-only removed wrong number of rows: got %d remaining, want 46", cnt)
	}
}

// ---------- Get ----------

func TestSQLite_Get_WithFilters(t *testing.T) {
	recreateTestStructTableSQLite()

	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}

	testStructs, _, err := testCRUDSQLite.Get(context.Background(), func() interface{} {
		return &test.TestStruct{}
	}, GetOptions{
		Order:  []string{"Age", "asc", "Price", "asc"},
		Limit:  10,
		Offset: 20,
		Filters: &sqlfilters.Filters{
			"Price":        {Op: sqlfilters.OpEqual, Val: 444},
			"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "primary@example.com"},
			sqlfilters.RawKey: {
				Op: sqlfilters.OpAND,
				Val: []interface{}{
					".Price > ? AND .Price NOT IN (?)",
					-200,
					[]int{9999, 9998, 9997},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SQLite Get with filters failed: %s", err.Error())
	}
	if len(testStructs) != 10 {
		t.Fatalf("SQLite Get returned wrong number of results: got %d, want 10", len(testStructs))
	}
	if testStructs[2].(*test.TestStruct).Age != 53 {
		t.Fatalf("SQLite Get returned wrong ordering: got Age=%d, want 53", testStructs[2].(*test.TestStruct).Age)
	}
}

func TestSQLite_Get_WithStringFilters(t *testing.T) {
	recreateTestStructTableSQLite()

	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}

	testStructs, _, err := testCRUDSQLite.Get(context.Background(), func() interface{} {
		return &test.TestStruct{}
	}, GetOptions{
		Order:  []string{"Age", "asc", "Price", "asc"},
		Limit:  10,
		Offset: 20,
		Filters: &sqlfilters.Filters{
			"Price":        {Op: sqlfilters.OpEqual, Val: "444"},
			"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "primary@example.com"},
			sqlfilters.RawKey: {
				Op: sqlfilters.OpAND,
				Val: []interface{}{
					".Price > ? AND .Price NOT IN (?)",
					-200,
					[]int{9999, 9998, 9997},
				},
			},
		},
		ConvertFiltersFromString: true,
	})
	if err != nil {
		t.Fatalf("SQLite Get with string filters failed: %s", err.Error())
	}
	if len(testStructs) != 10 {
		t.Fatalf("SQLite Get with string filters returned wrong count: got %d, want 10", len(testStructs))
	}
	if testStructs[2].(*test.TestStruct).Age != 53 {
		t.Fatalf("SQLite Get with string filters returned wrong ordering: got Age=%d, want 53", testStructs[2].(*test.TestStruct).Age)
	}
}

func TestSQLite_Get_WithoutFilters(t *testing.T) {
	recreateTestStructTableSQLite()

	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}

	testStructs, _, err := testCRUDSQLite.Get(context.Background(), func() interface{} {
		return &test.TestStruct{}
	}, GetOptions{
		Order:  []string{"Age", "asc"},
		Limit:  13,
		Offset: 14,
	})
	if err != nil {
		t.Fatalf("SQLite Get without filters failed: %s", err.Error())
	}
	if len(testStructs) != 13 {
		t.Fatalf("SQLite Get without filters returned wrong count: got %d, want 13", len(testStructs))
	}
	if testStructs[2].(*test.TestStruct).Age != 47 {
		t.Fatalf("SQLite Get without filters returned wrong ordering: got Age=%d, want 47", testStructs[2].(*test.TestStruct).Age)
	}
}

// ---------- GetCount ----------

func TestSQLite_GetCount_WithFilters(t *testing.T) {
	recreateTestStructTableSQLite()

	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}

	cnt, err := testCRUDSQLite.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{
		Filters: &sqlfilters.Filters{
			"Price":        {Op: sqlfilters.OpEqual, Val: 444},
			"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "primary@example.com"},
		},
	})
	if err != nil {
		t.Fatalf("SQLite GetCount with filters failed: %s", err.Error())
	}
	if cnt != 150 {
		t.Fatalf("SQLite GetCount returned wrong count: got %d, want 150", cnt)
	}
}

func TestSQLite_GetCount_WithRawQuery(t *testing.T) {
	recreateTestStructTableSQLite()

	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}

	cnt, err := testCRUDSQLite.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{
		Filters: &sqlfilters.Filters{
			"Price":        {Op: sqlfilters.OpEqual, Val: 444},
			"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "primary@example.com"},
			sqlfilters.RawKey: {
				Op: sqlfilters.OpOR,
				Val: []interface{}{
					".PrimaryEmail = ? AND .Age IN (?)",
					"another@example.com",
					[]int{32, 33, 34},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SQLite GetCount with raw query failed: %s", err.Error())
	}
	if cnt != 153 {
		t.Fatalf("SQLite GetCount with raw query returned wrong count: got %d, want 153", cnt)
	}
}

// ---------- UpdateMultiple ----------

func TestSQLite_UpdateMultiple(t *testing.T) {
	recreateTestStructTableSQLite()

	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		ts.PrimaryEmail = "changeme@example.com"
		_ = testCRUDSQLite.Save(context.Background(), ts, SaveOptions{})
	}

	err := testCRUDSQLite.UpdateMultiple(context.Background(), &test.TestStruct{}, map[string]interface{}{
		"PrimaryEmail": "newemail@example.com",
		"Age":          98,
	}, UpdateMultipleOptions{
		Filters: &sqlfilters.Filters{
			"Price":        {Op: sqlfilters.OpEqual, Val: 444},
			"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "changeme@example.com"},
		},
	})
	if err != nil {
		t.Fatalf("SQLite UpdateMultiple failed: %s", err.Error())
	}

	cnt, _ := testCRUDSQLite.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{
		Filters: &sqlfilters.Filters{
			"PrimaryEmail": {Op: sqlfilters.OpEqual, Val: "newemail@example.com"},
			"Age":          {Op: sqlfilters.OpEqual, Val: 98},
		},
	})
	if cnt != 150 {
		t.Fatalf("SQLite UpdateMultiple updated wrong number of rows: got %d with new values, want 150", cnt)
	}
}

func TestSQLite_UpdateMultiple_InvalidValues(t *testing.T) {
	recreateTestStructTableSQLite()

	for i := 1; i < 11; i++ {
		_ = testCRUDSQLite.Save(context.Background(), test.TestStructWithData(), SaveOptions{})
	}

	err := testCRUDSQLite.UpdateMultiple(context.Background(), &test.TestStruct{}, map[string]interface{}{
		"FirstName": "x",
		"Age":       "notanint",
	}, UpdateMultipleOptions{})
	fatalIfNotViolations(t, err, map[string]string{
		"FirstName": "min",
		"Age":       ViolationType,
	})

	cnt, _ := testCRUDSQLite.GetCount(context.Background(), &test.TestStruct{}, GetCountOptions{
		Filters: &sqlfilters.Filters{
			"FirstName": {Op: sqlfilters.OpEqual, Val: "John"},
			"Age":       {Op: sqlfilters.OpEqual, Val: 37},
		},
	})
	if cnt != 10 {
		t.Fatalf("SQLite UpdateMultiple updated rows with invalid values, got %d unchanged rows, want 10", cnt)
	}
}

// ---------- CreateTable with indexes ----------

type TestStructWithIndexes struct {
	ID        uint64
	Email     string `crud:"idx"`
	FirstName string `crud:"idx:name_idx"`
	LastName  string `crud:"idx:name_idx"`
	Code      string `crud:"uidx:code_idx"`
}

func TestSQLite_CreateTable_WithIndexes(t *testing.T) {
	obj := &TestStructWithIndexes{}

	_ = testCRUDSQLite.DropTable(context.Background(), obj)
	err := testCRUDSQLite.CreateTable(context.Background(), obj)
	if err != nil {
		t.Fatalf("SQLite CreateTable with indexes failed: %s", err.Error())
	}
	defer func() { _ = testCRUDSQLite.DropTable(context.Background(), obj) }()

	rows, err := testDBSQLite.Query(`PRAGMA index_list(test_struct_with_indexes)`)
	if err != nil {
		t.Fatalf("SQLite failed to list indexes: %s", err.Error())
	}
	defer rows.Close()

	unique := map[string]bool{}
	for rows.Next() {
		var seq, isUnique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &isUnique, &origin, &partial); err != nil {
			t.Fatalf("SQLite failed to scan index_list row: %s", err.Error())
		}
		unique[name] = isUnique == 1
	}

	wantIndexes := map[string]bool{
		"idx_test_struct_with_indexes_email":    false,
		"idx_test_struct_with_indexes_name_idx": false,
		"idx_test_struct_with_indexes_code_idx": true,
	}
	for name, wantUnique := range wantIndexes {
		gotUnique, ok := unique[name]
		if !ok {
			t.Fatalf("SQLite CreateTable did not create expected index %q (got %v)", name, unique)
		}
		if gotUnique != wantUnique {
			t.Fatalf("SQLite index %q unique=%v, want %v", name, gotUnique, wantUnique)
		}
	}

	infoRows, err := testDBSQLite.Query(`PRAGMA index_info(idx_test_struct_with_indexes_name_idx)`)
	if err != nil {
		t.Fatalf("SQLite failed to inspect composite index: %s", err.Error())
	}
	defer infoRows.Close()

	var cols []string
	for infoRows.Next() {
		var seqno, cid int
		var colName string
		if err := infoRows.Scan(&seqno, &cid, &colName); err != nil {
			t.Fatalf("SQLite failed to scan index_info row: %s", err.Error())
		}
		cols = append(cols, colName)
	}
	if len(cols) != 2 || cols[0] != "first_name" || cols[1] != "last_name" {
		t.Fatalf("SQLite composite index columns = %v, want [first_name last_name]", cols)
	}
}
