package gocrud

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type PreCustomer struct {
	ID     uint64
	Name   string
	Orders []PreOrder `crud:"rel:CustomerID"`
}

type PreShipper struct {
	ID     uint64
	Name   string
	APIKey string `crud:"pass"`
}

type PreOrder struct {
	ID         uint64
	CustomerID uint64
	ShipperID  *uint64
	Status     string
	Customer   *PreCustomer `crud:"rel:CustomerID"`
	Shipper    *PreShipper  `crud:"rel:ShipperID"`
	Items      []*PreItem   `crud:"rel:OrderID"`
	Notes      []PreNote    `crud:"rel:OrderID"`
}

type PreItem struct {
	ID      uint64
	OrderID uint64
	Product string
}

type PreNote struct {
	ID      uint64
	OrderID *uint64
	At      time.Time
	Text    string
}

// countingConn counts the queries run on a connection.
type countingConn struct {
	DBTX
	queries int
}

func (c *countingConn) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	c.queries++
	return c.DBTX.QueryContext(ctx, query, args...)
}

func (c *countingConn) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	c.queries++
	return c.DBTX.QueryRowContext(ctx, query, args...)
}

func withCountingConn(crud *CRUD) (*CRUD, *countingConn) {
	conn := &countingConn{DBTX: crud.conn}
	counted := *crud
	counted.conn = conn
	return &counted, conn
}

func createPreloadData(t *testing.T, crud *CRUD) {
	t.Helper()
	ctx := context.Background()

	for _, obj := range []interface{}{&PreCustomer{}, &PreShipper{}, &PreOrder{}, &PreItem{}, &PreNote{}} {
		_ = crud.DropTable(ctx, obj)
		if err := crud.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed: %s", err.Error())
		}
	}

	save := func(obj interface{}) {
		if err := crud.Save(ctx, obj, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	save(&PreCustomer{Name: "Acme"})
	save(&PreCustomer{Name: "Globex"})
	save(&PreShipper{Name: "DHL", APIKey: "secret"})

	shipperID := uint64(1)
	missingShipperID := uint64(99)
	save(&PreOrder{CustomerID: 1, ShipperID: &shipperID, Status: "placed"})
	save(&PreOrder{CustomerID: 1, Status: "paid"})
	save(&PreOrder{CustomerID: 2, ShipperID: &missingShipperID, Status: "placed"})
	save(&PreOrder{CustomerID: 0, Status: "draft"})

	save(&PreItem{OrderID: 1, Product: "Keyboard"})
	save(&PreItem{OrderID: 2, Product: "Mouse"})
	save(&PreItem{OrderID: 1, Product: "Cable"})

	orderID := uint64(1)
	warsaw := mustLoadLocation(t, "Europe/Warsaw")
	save(&PreNote{OrderID: &orderID, At: time.Date(2024, 5, 1, 12, 0, 0, 0, warsaw), Text: "Fragile"})
	save(&PreNote{Text: "Not attached"})
}

func products(items []*PreItem) []string {
	got := make([]string, len(items))
	for i, item := range items {
		got[i] = item.Product
	}
	return got
}

func testPreloadGet(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)

	counted, conn := withCountingConn(crud)
	orders, err := Get[PreOrder](context.Background(), counted, GetOptions{
		Order:   []string{"ID", "asc"},
		Preload: []string{"Customer", "Shipper", "Items", "Notes"},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if conn.queries != 5 {
		t.Fatalf("Get ran %d queries for 4 orders with 4 relations, want 5", conn.queries)
	}

	if orders[0].Customer == nil || orders[0].Customer.Name != "Acme" || orders[1].Customer != orders[0].Customer {
		t.Fatal("Get did not set the same customer for the orders of Acme")
	}
	if orders[2].Customer == nil || orders[2].Customer.Name != "Globex" || orders[3].Customer != nil {
		t.Fatal("Get did not set the customers of the other orders")
	}

	if orders[0].Shipper == nil || orders[0].Shipper.Name != "DHL" || orders[0].Shipper.APIKey != "" {
		t.Fatalf("Get set shipper %+v, want DHL without the password", orders[0].Shipper)
	}
	if orders[1].Shipper != nil || orders[2].Shipper != nil {
		t.Fatal("Get set a shipper for an order without it, or with a shipper that does not exist")
	}

	if got := products(orders[0].Items); len(got) != 2 || got[0] != "Keyboard" || got[1] != "Cable" {
		t.Fatalf("Get set items %v, want [Keyboard Cable]", got)
	}
	if got := products(orders[1].Items); len(got) != 1 || got[0] != "Mouse" {
		t.Fatalf("Get set items %v, want [Mouse]", got)
	}
	if orders[2].Items == nil || len(orders[2].Items) != 0 || orders[3].Items == nil {
		t.Fatal("Get did not set an empty slice for orders without items")
	}

	if len(orders[0].Notes) != 1 || orders[0].Notes[0].Text != "Fragile" || orders[0].Notes[0].At.Location() != time.UTC {
		t.Fatalf("Get set notes %+v, want the Fragile note in UTC", orders[0].Notes)
	}
	if len(orders[1].Notes) != 0 {
		t.Fatalf("Get set notes %+v, want none", orders[1].Notes)
	}
}

func testPreloadHasManyValues(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)

	customers, err := Get[PreCustomer](context.Background(), crud, GetOptions{
		Order:   []string{"ID", "asc"},
		Preload: []string{"Orders"},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if len(customers[0].Orders) != 2 || customers[0].Orders[0].Status != "placed" || customers[0].Orders[1].Status != "paid" ||
		len(customers[1].Orders) != 1 || customers[1].Orders[0].Customer != nil {
		t.Fatalf("Get set orders %+v and %+v", customers[0].Orders, customers[1].Orders)
	}
}

func testPreloadChunks(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)

	defaultChunkSize := preloadChunkSize
	preloadChunkSize = 2
	defer func() { preloadChunkSize = defaultChunkSize }()

	counted, conn := withCountingConn(crud)
	orders, err := Get[PreOrder](context.Background(), counted, GetOptions{
		Order:   []string{"ID", "asc"},
		Preload: []string{"Customer", "Items"},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	// 2 customer keys in 1 chunk, 4 order keys in 2 chunks
	if conn.queries != 4 {
		t.Fatalf("Get ran %d queries, want 4", conn.queries)
	}
	if orders[2].Customer.Name != "Globex" || len(orders[0].Items) != 2 || len(orders[1].Items) != 1 {
		t.Fatal("Get did not set the related objects loaded in chunks")
	}
}

func testPreloadLoad(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)
	ctx := context.Background()

	order, err := Load[PreOrder](ctx, crud, 1, "Customer", "Items")
	if err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if order.Customer == nil || order.Customer.Name != "Acme" || len(order.Items) != 2 || order.Shipper != nil || order.Notes != nil {
		t.Fatalf("Load returned %+v, want the customer and the items only", *order)
	}

	untyped := &PreOrder{}
	if _, err := crud.Load(ctx, untyped, 3, LoadOptions{Preload: []string{"Customer"}}); err != nil || untyped.Customer.Name != "Globex" {
		t.Fatalf("Load returned %v and customer %+v, want Globex", err, untyped.Customer)
	}

	counted, conn := withCountingConn(crud)
	if _, err := Load[PreOrder](ctx, counted, 99, "Customer"); !errors.Is(err, ErrNotFound) || conn.queries != 1 {
		t.Fatalf("Load returned err %v after %d queries, want %v after 1", err, conn.queries, ErrNotFound)
	}
}

func testPreloadInTx(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)
	ctx := context.Background()
	errAbort := errors.New("abort")

	err := crud.RunInTx(ctx, func(tx *CRUD) error {
		if err := tx.Save(ctx, &PreItem{OrderID: 2, Product: "Uncommitted"}, SaveOptions{}); err != nil {
			return err
		}

		orders, err := Get[PreOrder](ctx, tx, GetOptions{
			Filters: &sqlfilters.Filters{"ID": {Op: sqlfilters.OpEqual, Val: uint64(2)}},
			Preload: []string{"Customer", "Items"},
		})
		if err != nil {
			return err
		}
		if got := products(orders[0].Items); len(got) != 2 || got[1] != "Uncommitted" {
			t.Fatalf("Get in the transaction set items %v, want the uncommitted item", got)
		}
		return errAbort
	})
	if !errors.Is(err, errAbort) {
		t.Fatalf("RunInTx returned %v, want %v", err, errAbort)
	}
}

func testPreloadTransform(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)

	objs, _, err := crud.Get(context.Background(), func() interface{} { return &PreOrder{} }, GetOptions{
		Order:   []string{"ID", "asc"},
		Limit:   1,
		Preload: []string{"Customer"},
		RowObjTransformFunc: func(obj interface{}) interface{} {
			return obj.(*PreOrder).Status + " by " + obj.(*PreOrder).Customer.Name
		},
	})
	if err != nil || len(objs) != 1 || objs[0] != "placed by Acme" {
		t.Fatalf("Get returned %v, %v, want the transform to see the customer", objs, err)
	}
}

func testPreloadInvalid(t *testing.T, crud *CRUD) {
	createPreloadData(t, crud)

	for _, name := range []string{"Status", "Warehouse", "Items.Product", "customer"} {
		_, err := Get[PreOrder](context.Background(), crud, GetOptions{Preload: []string{name}})
		if !errors.Is(err, ErrPreload) {
			t.Fatalf("Get with Preload %q returned err %v, want %v", name, err, ErrPreload)
		}
	}

	counted, conn := withCountingConn(crud)
	orders, err := Get[PreOrder](context.Background(), counted, GetOptions{
		Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "shipped"}},
		Preload: []string{"Customer", "Items"},
	})
	if err != nil || len(orders) != 0 || conn.queries != 1 {
		t.Fatalf("Get returned %v, %v after %d queries, want no orders after 1 query", orders, err, conn.queries)
	}
}

func TestPreload_Get(t *testing.T)        { testPreloadGet(t, testCRUD) }
func TestSQLite_Preload_Get(t *testing.T) { testPreloadGet(t, testCRUDSQLite) }

func TestPreload_HasManyValues(t *testing.T)        { testPreloadHasManyValues(t, testCRUD) }
func TestSQLite_Preload_HasManyValues(t *testing.T) { testPreloadHasManyValues(t, testCRUDSQLite) }

func TestPreload_Chunks(t *testing.T)        { testPreloadChunks(t, testCRUD) }
func TestSQLite_Preload_Chunks(t *testing.T) { testPreloadChunks(t, testCRUDSQLite) }

func TestPreload_Load(t *testing.T)        { testPreloadLoad(t, testCRUD) }
func TestSQLite_Preload_Load(t *testing.T) { testPreloadLoad(t, testCRUDSQLite) }

func TestPreload_InTx(t *testing.T)        { testPreloadInTx(t, testCRUD) }
func TestSQLite_Preload_InTx(t *testing.T) { testPreloadInTx(t, testCRUDSQLite) }

func TestPreload_Transform(t *testing.T)        { testPreloadTransform(t, testCRUD) }
func TestSQLite_Preload_Transform(t *testing.T) { testPreloadTransform(t, testCRUDSQLite) }

func TestPreload_Invalid(t *testing.T)        { testPreloadInvalid(t, testCRUD) }
func TestSQLite_Preload_Invalid(t *testing.T) { testPreloadInvalid(t, testCRUDSQLite) }
