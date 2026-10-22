package gocrud

import (
	"context"
	"testing"
)

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
