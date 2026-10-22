package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud"
)

type SvcCustomer struct {
	ID   uint64
	Name string
}

type SvcOrder struct {
	ID         uint64
	CustomerID uint64 `crud:"fk:svc_customer on_delete:cascade"`
}

type SvcItem struct {
	ID      uint64
	OrderID uint64 `crud:"fk:svc_order on_delete:cascade"`
}

type SvcCategory struct {
	ID       uint64
	ParentID *uint64 `crud:"fk:svc_category on_delete:set_null"`
	Name     string
}

type SvcInvoice struct {
	ID         uint64
	ExternalID uint64 `crud:"fk:svc_external"`
}

type SvcCycleA struct {
	ID  uint64
	BID uint64 `crud:"fk:svc_cycle_b"`
}

type SvcCycleB struct {
	ID  uint64
	AID uint64 `crud:"fk:svc_cycle_a"`
}

func newOrderedService() *CRUD {
	return New(map[string]func() interface{}{
		"a_items":      func() interface{} { return &SvcItem{} },
		"b_orders":     func() interface{} { return &SvcOrder{} },
		"c_customers":  func() interface{} { return &SvcCustomer{} },
		"d_categories": func() interface{} { return &SvcCategory{} },
	}, testDB, gocrud.DialectPostgres)
}

func dropOrderedTables() {
	for _, obj := range []interface{}{&SvcItem{}, &SvcOrder{}, &SvcCustomer{}, &SvcCategory{}} {
		_ = testCRUD.DropTable(context.Background(), obj)
	}
}

func TestTableKeys_Order(t *testing.T) {
	keys, err := newOrderedService().tableKeys()
	if err != nil {
		t.Fatalf("tableKeys failed: %s", err.Error())
	}
	if want := []string{"c_customers", "b_orders", "a_items", "d_categories"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("tableKeys returned %v, want %v", keys, want)
	}

	svc := New(map[string]func() interface{}{
		"invoices": func() interface{} { return &SvcInvoice{} },
	}, testDB, gocrud.DialectPostgres)
	if keys, err := svc.tableKeys(); err != nil || !reflect.DeepEqual(keys, []string{"invoices"}) {
		t.Fatalf("tableKeys returned %v, %v for a table that is not in the registry, want [invoices]", keys, err)
	}
}

func TestCreateTables_ForeignKeyOrder(t *testing.T) {
	dropOrderedTables()

	if err := newOrderedService().CreateTables(context.Background()); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}
}

func TestMigrate_ForeignKeyOrder(t *testing.T) {
	dropOrderedTables()

	migrations, err := newOrderedService().Migrate(context.Background())
	if err != nil {
		t.Fatalf("Migrate failed: %s", err.Error())
	}

	var tables []string
	for _, migration := range migrations {
		tables = append(tables, migration.Table)
	}
	if want := []string{"svc_customer", "svc_order", "svc_item", "svc_category"}; !reflect.DeepEqual(tables, want) {
		t.Fatalf("Migrate migrated %v, want %v", tables, want)
	}
}

func TestCreateTables_ForeignKeyCycle(t *testing.T) {
	svc := New(map[string]func() interface{}{
		"a": func() interface{} { return &SvcCycleA{} },
		"b": func() interface{} { return &SvcCycleB{} },
	}, testDB, gocrud.DialectPostgres)

	if err := svc.CreateTables(context.Background()); !errors.Is(err, ErrForeignKeyCycle) {
		t.Fatalf("CreateTables returned err %v, want %v", err, ErrForeignKeyCycle)
	}
	if _, err := svc.Migrate(context.Background()); !errors.Is(err, ErrForeignKeyCycle) {
		t.Fatalf("Migrate returned err %v, want %v", err, ErrForeignKeyCycle)
	}
}
