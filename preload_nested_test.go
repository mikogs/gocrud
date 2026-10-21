package gocrud

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type NestSupplier struct {
	ID   uint64
	Name string
}

type NestProduct struct {
	ID         uint64
	Name       string
	SupplierID uint64
	Supplier   *NestSupplier `crud:"rel:SupplierID"`
}

type NestItem struct {
	ID        uint64
	OrderID   uint64
	ProductID uint64
	Product   *NestProduct `crud:"rel:ProductID"`
}

type NestOrder struct {
	ID         uint64
	CustomerID uint64
	Customer   *NestCustomer `crud:"rel:CustomerID"`
	Items      []NestItem    `crud:"rel:OrderID"`
}

type NestCustomer struct {
	ID     uint64
	Name   string
	Orders []*NestOrder `crud:"rel:CustomerID"`
}

func createNestedPreloadData(t *testing.T, crud *CRUD) {
	t.Helper()
	ctx := context.Background()

	for _, obj := range []interface{}{&NestSupplier{}, &NestProduct{}, &NestItem{}, &NestOrder{}, &NestCustomer{}} {
		_ = crud.DropTable(ctx, obj)
		if err := crud.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed: %s", err.Error())
		}
	}

	for _, obj := range []interface{}{
		&NestSupplier{Name: "Logitech"}, &NestSupplier{Name: "Cherry"},
		&NestProduct{Name: "Mouse", SupplierID: 1}, &NestProduct{Name: "Keyboard", SupplierID: 2}, &NestProduct{Name: "Webcam", SupplierID: 1},
		&NestCustomer{Name: "Acme"}, &NestCustomer{Name: "Globex"},
		&NestOrder{CustomerID: 1}, &NestOrder{CustomerID: 1}, &NestOrder{CustomerID: 2},
		&NestItem{OrderID: 1, ProductID: 1}, &NestItem{OrderID: 1, ProductID: 2}, &NestItem{OrderID: 2, ProductID: 1}, &NestItem{OrderID: 3, ProductID: 3},
	} {
		if err := crud.Save(ctx, obj, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}
}

func testNestedPreload(t *testing.T, crud *CRUD) {
	createNestedPreloadData(t, crud)

	counted, conn := withCountingConn(crud)
	orders, err := Get[NestOrder](context.Background(), counted, GetOptions{
		Order:   []string{"ID", "asc"},
		Preload: []string{"Items.Product.Supplier", "Customer"},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if conn.queries != 5 {
		t.Fatalf("Get ran %d queries for orders, items, products, suppliers and customers, want 5", conn.queries)
	}

	if len(orders[0].Items) != 2 || orders[0].Items[1].Product == nil || orders[0].Items[1].Product.Name != "Keyboard" ||
		orders[0].Items[1].Product.Supplier == nil || orders[0].Items[1].Product.Supplier.Name != "Cherry" {
		t.Fatalf("Get set items %+v, want the products and their suppliers", orders[0].Items)
	}
	if orders[0].Items[0].Product != orders[1].Items[0].Product {
		t.Fatal("Get did not set the same product for the items of the same product")
	}
	if orders[2].Items[0].Product.Name != "Webcam" || orders[2].Items[0].Product.Supplier.Name != "Logitech" || orders[2].Customer.Name != "Globex" {
		t.Fatalf("Get set %+v, want the Webcam from Logitech for Globex", orders[2])
	}
}

func testNestedPreloadBackReference(t *testing.T, crud *CRUD) {
	createNestedPreloadData(t, crud)

	counted, conn := withCountingConn(crud)
	customers, err := Get[NestCustomer](context.Background(), counted, GetOptions{
		Order:   []string{"ID", "asc"},
		Preload: []string{"Orders.Customer", "Orders.Items", "Orders"},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if conn.queries != 4 {
		t.Fatalf("Get ran %d queries for customers, orders, their customers and items, want 4", conn.queries)
	}

	acme := customers[0]
	if len(acme.Orders) != 2 || acme.Orders[0].Customer == nil || acme.Orders[0].Customer.Name != "Acme" ||
		len(acme.Orders[0].Items) != 2 || acme.Orders[0].Items[0].Product != nil {
		t.Fatalf("Get set orders %+v, want their customer and items without products", acme.Orders)
	}
}

func testNestedPreloadLoad(t *testing.T, crud *CRUD) {
	createNestedPreloadData(t, crud)

	order, err := Load[NestOrder](context.Background(), crud, 1, "Items.Product.Supplier")
	if err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if len(order.Items) != 2 || order.Items[0].Product.Supplier.Name != "Logitech" || order.Customer != nil {
		t.Fatalf("Load returned %+v, want the items with products and suppliers only", *order)
	}
}

func testNestedPreloadInvalid(t *testing.T, crud *CRUD) {
	createNestedPreloadData(t, crud)

	tests := map[string]string{
		"Items.Warehouse":      "Items.Warehouse: Warehouse is not a relation field of NestItem",
		"Items.ProductID":      "Items.ProductID: ProductID is not a relation field of NestItem",
		"Customer.Orders.Nope": "Customer.Orders.Nope: Nope is not a relation field of NestOrder",
		"Items.":               "Items.: the path has an empty relation name",
		".Items":               ".Items: the path has an empty relation name",
		"Items..Product":       "Items..Product: the path has an empty relation name",
	}

	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			counted, conn := withCountingConn(crud)
			_, err := Get[NestOrder](context.Background(), counted, GetOptions{Preload: []string{"Customer", path}})
			if !errors.Is(err, PreloadError) || !strings.Contains(err.Error(), want) {
				t.Fatalf("Get returned err %v, want %v with %q", err, PreloadError, want)
			}
			if conn.queries != 0 {
				t.Fatalf("Get ran %d queries for an invalid preload path, want 0", conn.queries)
			}

			if _, err := Load[NestOrder](context.Background(), counted, 1, path); !errors.Is(err, PreloadError) || conn.queries != 0 {
				t.Fatalf("Load returned err %v after %d queries, want %v after 0", err, conn.queries, PreloadError)
			}
		})
	}
}

func TestPreload_Nested(t *testing.T)        { testNestedPreload(t, testCRUD) }
func TestSQLite_Preload_Nested(t *testing.T) { testNestedPreload(t, testCRUDSQLite) }

func TestPreload_NestedBackReference(t *testing.T) { testNestedPreloadBackReference(t, testCRUD) }
func TestSQLite_Preload_NestedBackReference(t *testing.T) {
	testNestedPreloadBackReference(t, testCRUDSQLite)
}

func TestPreload_NestedLoad(t *testing.T)        { testNestedPreloadLoad(t, testCRUD) }
func TestSQLite_Preload_NestedLoad(t *testing.T) { testNestedPreloadLoad(t, testCRUDSQLite) }

func TestPreload_NestedInvalid(t *testing.T)        { testNestedPreloadInvalid(t, testCRUD) }
func TestSQLite_Preload_NestedInvalid(t *testing.T) { testNestedPreloadInvalid(t, testCRUDSQLite) }
