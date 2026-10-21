package gocrud

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

type FkCustomer struct {
	ID   uint64
	Name string
}

type FkShipper struct {
	ID   uint64
	Name string
}

type FkOrder struct {
	ID         uint64
	CustomerID uint64  `crud:"idx fk:fk_customer on_delete:cascade"`
	ShipperID  *uint64 `crud:"fk:fk_shipper on_delete:set_null"`
	Status     string
	Customer   *FkCustomer `crud:"rel:CustomerID"`
}

type FkInvoice struct {
	ID      uint64
	OrderID uint64 `crud:"fk:fk_order on_delete:restrict"`
}

type FkNote struct {
	ID      uint64
	OrderID uint64 `crud:"fk:fk_order"`
}

// newSQLiteForeignKeysCRUD returns a CRUD for a SQLite database that enforces foreign keys.
func newSQLiteForeignKeysCRUD(t *testing.T) *CRUD {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "fk.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("opening the database failed: %s", err.Error())
	}
	t.Cleanup(func() { _ = db.Close() })

	return New(db, Options{Dialect: DialectSQLite})
}

func createForeignKeyTables(t *testing.T, crud *CRUD) {
	t.Helper()
	ctx := context.Background()

	for _, obj := range []interface{}{&FkNote{}, &FkInvoice{}, &FkOrder{}, &FkShipper{}, &FkCustomer{}} {
		_ = crud.DropTable(ctx, obj)
	}
	for _, obj := range []interface{}{&FkCustomer{}, &FkShipper{}, &FkOrder{}, &FkInvoice{}, &FkNote{}} {
		if err := crud.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed for %T: %s", obj, err.Error())
		}
	}

	for _, obj := range []interface{}{&FkCustomer{Name: "Acme"}, &FkCustomer{Name: "Globex"}, &FkShipper{Name: "DHL"}} {
		if err := crud.Save(ctx, obj, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	shipperID := uint64(1)
	for _, order := range []*FkOrder{
		{CustomerID: 1, ShipperID: &shipperID, Status: "placed"},
		{CustomerID: 1, Status: "paid"},
		{CustomerID: 2, ShipperID: &shipperID, Status: "placed"},
	} {
		if err := crud.Save(ctx, order, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}
}

func fatalIfNotForeignKeyViolation(t *testing.T, err error) {
	t.Helper()

	if !errors.Is(err, ForeignKeyViolationError) {
		t.Fatalf("got err %v, want %v", err, ForeignKeyViolationError)
	}
}

func orderIDs(t *testing.T, crud *CRUD) []uint64 {
	t.Helper()

	orders, err := Get[FkOrder](context.Background(), crud, GetOptions{Order: []string{"ID", "asc"}})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}

	ids := make([]uint64, len(orders))
	for i, order := range orders {
		ids[i] = order.ID
	}
	return ids
}

func testForeignKeysOnSave(t *testing.T, crud *CRUD) {
	createForeignKeyTables(t, crud)
	ctx := context.Background()

	fatalIfNotForeignKeyViolation(t, crud.Save(ctx, &FkOrder{CustomerID: 99, Status: "placed"}, SaveOptions{}))
	fatalIfNotForeignKeyViolation(t, crud.Save(ctx, &FkOrder{Status: "placed"}, SaveOptions{}))

	missingShipperID := uint64(99)
	fatalIfNotForeignKeyViolation(t, crud.Save(ctx, &FkOrder{CustomerID: 1, ShipperID: &missingShipperID}, SaveOptions{}))

	order, _ := Load[FkOrder](ctx, crud, 1)
	order.CustomerID = 99
	fatalIfNotForeignKeyViolation(t, crud.Save(ctx, order, SaveOptions{NoInsert: true}))

	err := UpdateMultiple[FkOrder](ctx, crud, map[string]interface{}{"CustomerID": uint64(99)}, UpdateMultipleOptions{})
	fatalIfNotForeignKeyViolation(t, err)

	if err := crud.Save(ctx, &FkInvoice{OrderID: 1}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed for a valid reference: %s", err.Error())
	}
}

func testForeignKeysOnDelete(t *testing.T, crud *CRUD) {
	createForeignKeyTables(t, crud)
	ctx := context.Background()

	// cascade
	acme, _ := Load[FkCustomer](ctx, crud, 1)
	if err := crud.Delete(ctx, acme, DeleteOptions{}); err != nil {
		t.Fatalf("Delete failed: %s", err.Error())
	}
	if ids := orderIDs(t, crud); !reflect.DeepEqual(ids, []uint64{3}) {
		t.Fatalf("orders %v are left after deleting the customer, want [3]", ids)
	}

	// set null
	if _, err := DeleteMultiple[FkShipper](ctx, crud, DeleteMultipleOptions{}); err != nil {
		t.Fatalf("DeleteMultiple failed: %s", err.Error())
	}
	order, err := Load[FkOrder](ctx, crud, 3, "Customer")
	if err != nil || order.ShipperID != nil || order.Customer.Name != "Globex" {
		t.Fatalf("Load returned %+v, %v, want the order without a shipper", order, err)
	}

	// restrict and no action
	for _, obj := range []interface{}{&FkInvoice{OrderID: 3}, &FkNote{OrderID: 3}} {
		if err := crud.Save(ctx, obj, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
		fatalIfNotForeignKeyViolation(t, crud.Delete(ctx, &FkOrder{ID: 3}, DeleteOptions{}))
		_, err := DeleteMultiple[FkCustomer](ctx, crud, DeleteMultipleOptions{})
		fatalIfNotForeignKeyViolation(t, err)
		if err := crud.Delete(ctx, obj, DeleteOptions{}); err != nil {
			t.Fatalf("Delete failed: %s", err.Error())
		}
	}

	if _, err := DeleteMultiple[FkCustomer](ctx, crud, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "Globex"}},
	}); err != nil {
		t.Fatalf("DeleteMultiple failed: %s", err.Error())
	}
	if ids := orderIDs(t, crud); len(ids) != 0 {
		t.Fatalf("orders %v are left after deleting all the customers, want none", ids)
	}
}

func testForeignKeysMigrate(t *testing.T, crud *CRUD) {
	createForeignKeyTables(t, crud)

	for _, obj := range []interface{}{&FkCustomer{}, &FkShipper{}, &FkOrder{}, &FkInvoice{}, &FkNote{}} {
		if migration := mustMigrate(t, crud, obj); len(migration.Differences) != 0 {
			t.Fatalf("Migrate returned differences %v for %T, want none", migration.Differences, obj)
		}
	}
}

type FkReview struct {
	ID         uint64
	CustomerID uint64
	Text       string
}

type FkReview_V2 struct {
	ID         uint64
	CustomerID uint64 `crud:"fk:fk_customer"`
	Text       string
	ShipperID  *uint64 `crud:"fk:fk_shipper on_delete:set_null"`
}

type FkReview_V3 struct {
	ID         uint64
	CustomerID uint64 `crud:"fk:fk_customer"`
	Text       string
	OrderID    uint64 `crud:"fk:fk_order"`
}

type FkInvoice_V2 struct {
	ID      uint64
	OrderID uint64 `crud:"fk:fk_shipper on_delete:cascade"`
}

func testForeignKeysMigrateChanges(t *testing.T, crud *CRUD) {
	createForeignKeyTables(t, crud)
	ctx := context.Background()

	_ = DropTable[FkReview](ctx, crud)
	mustMigrate(t, crud, &FkReview{})
	if err := crud.Save(ctx, &FkReview{CustomerID: 1, Text: "Great"}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	migration := mustMigrate(t, crud, &FkReview_V2{})
	wantDifferences := []string{`column "customer_id" has no foreign key, the field references table "fk_customer"`}
	if !reflect.DeepEqual(migration.AddedColumns, []string{"shipper_id"}) || !reflect.DeepEqual(migration.Differences, wantDifferences) {
		t.Fatalf("Migrate returned %+v, want shipper_id added and %v", *migration, wantDifferences)
	}

	missingShipperID := uint64(99)
	fatalIfNotForeignKeyViolation(t, crud.Save(ctx, &FkReview_V2{CustomerID: 1, ShipperID: &missingShipperID}, SaveOptions{}))

	// a column that cannot be NULL gets 0 in the existing rows, which does not reference anything
	if _, err := crud.Migrate(ctx, &FkReview_V3{}); err == nil {
		t.Fatal("Migrate did not fail to add a foreign key column that cannot be NULL to a table with rows")
	}
	columns, _ := crud.databaseColumns(ctx, "fk_review")
	if _, ok := columns["order_id"]; ok {
		t.Fatal("Migrate did not roll back the column")
	}

	migration = mustMigrate(t, crud, &FkInvoice_V2{})
	wantDifferences = []string{`column "order_id" references table "fk_order", the field references table "fk_shipper"`}
	if !reflect.DeepEqual(migration.Differences, wantDifferences) {
		t.Fatalf("Migrate returned differences %v, want %v", migration.Differences, wantDifferences)
	}

	type FkInvoice_V3 struct {
		ID      uint64
		OrderID uint64 `crud:"fk:fk_order on_delete:cascade"`
	}
	migration = mustMigrate(t, crud, &FkInvoice_V3{})
	wantDifferences = []string{`foreign key of column "order_id" has ON DELETE RESTRICT, the field needs CASCADE`}
	if !reflect.DeepEqual(migration.Differences, wantDifferences) {
		t.Fatalf("Migrate returned differences %v, want %v", migration.Differences, wantDifferences)
	}
}

func TestForeignKeys_OnSave(t *testing.T) { testForeignKeysOnSave(t, testCRUD) }
func TestSQLite_ForeignKeys_OnSave(t *testing.T) {
	testForeignKeysOnSave(t, newSQLiteForeignKeysCRUD(t))
}

func TestForeignKeys_OnDelete(t *testing.T) { testForeignKeysOnDelete(t, testCRUD) }
func TestSQLite_ForeignKeys_OnDelete(t *testing.T) {
	testForeignKeysOnDelete(t, newSQLiteForeignKeysCRUD(t))
}

func TestForeignKeys_Migrate(t *testing.T) { testForeignKeysMigrate(t, testCRUD) }
func TestSQLite_ForeignKeys_Migrate(t *testing.T) {
	testForeignKeysMigrate(t, newSQLiteForeignKeysCRUD(t))
}

func TestForeignKeys_MigrateChanges(t *testing.T) { testForeignKeysMigrateChanges(t, testCRUD) }
func TestSQLite_ForeignKeys_MigrateChanges(t *testing.T) {
	testForeignKeysMigrateChanges(t, newSQLiteForeignKeysCRUD(t))
}

func TestSQLite_ForeignKeys_NotEnforced(t *testing.T) {
	ctx := context.Background()
	for _, obj := range []interface{}{&FkNote{}, &FkInvoice{}, &FkOrder{}, &FkShipper{}, &FkCustomer{}} {
		_ = testCRUDSQLite.DropTable(ctx, obj)
	}
	mustMigrate(t, testCRUDSQLite, &FkCustomer{})
	mustMigrate(t, testCRUDSQLite, &FkShipper{})

	migration := mustMigrate(t, testCRUDSQLite, &FkOrder{})
	sort.Strings(migration.Differences)
	want := []string{"foreign keys are not enforced, open the database with the _pragma=foreign_keys(1) parameter"}
	if !migration.CreatedTable || !reflect.DeepEqual(migration.Differences, want) {
		t.Fatalf("Migrate returned %+v, want the table created and %v", *migration, want)
	}

	if migration := mustMigrate(t, testCRUDSQLite, &FkCustomer{}); len(migration.Differences) != 0 {
		t.Fatalf("Migrate returned differences %v for a struct without foreign keys, want none", migration.Differences)
	}
}
