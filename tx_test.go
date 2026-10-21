package gocrud

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/test"
)

type Product struct {
	ID    uint64 `json:"id"`
	Name  string `json:"name" validate:"required"`
	Price int64  `json:"price"`
	Stock int64  `json:"stock" validate:"min=0"`
}

type Order struct {
	ID         uint64 `json:"id"`
	CustomerID uint64 `json:"customer_id" crud:"idx"`
	Status     string `json:"status" validate:"required,oneof=placed paid shipped cancelled"`
	Total      int64  `json:"total"`
}

type OrderItem struct {
	ID        uint64 `json:"id"`
	OrderID   uint64 `json:"order_id" crud:"idx"`
	ProductID uint64 `json:"product_id"`
	Quantity  int64  `json:"quantity" validate:"min=1"`
	Price     int64  `json:"price"`
}

type CartItem struct {
	ProductID uint64
	Quantity  int64
}

var (
	ErrProductNotFound = errors.New("product not found")
	ErrOutOfStock      = errors.New("product out of stock")
)

func placeOrder(ctx context.Context, crud *CRUD, customerID uint64, cart []CartItem) (*Order, error) {
	order := &Order{CustomerID: customerID, Status: "placed"}

	err := crud.RunInTx(ctx, func(tx *CRUD) error {
		if err := tx.Save(ctx, order, SaveOptions{}); err != nil {
			return err
		}

		for _, item := range cart {
			product := &Product{}
			if err := tx.Load(ctx, product, strconv.FormatUint(item.ProductID, 10), LoadOptions{}).Error; err != nil {
				return err
			}
			if product.ID == 0 {
				return fmt.Errorf("%w: %d", ErrProductNotFound, item.ProductID)
			}

			res, err := tx.Tx().ExecContext(ctx,
				`UPDATE product SET stock = stock - $1 WHERE id = $2 AND stock >= $1`,
				item.Quantity, product.ID)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return fmt.Errorf("%w: %s", ErrOutOfStock, product.Name)
			}

			orderItem := &OrderItem{
				OrderID:   order.ID,
				ProductID: product.ID,
				Quantity:  item.Quantity,
				Price:     product.Price,
			}
			if err := tx.Save(ctx, orderItem, SaveOptions{}); err != nil {
				return err
			}

			order.Total += product.Price * item.Quantity
		}

		return tx.Save(ctx, order, SaveOptions{NoInsert: true})
	})
	if err != nil {
		return nil, err
	}

	return order, nil
}

func createShopTables(t *testing.T, crud *CRUD) (*Product, *Product) {
	t.Helper()
	ctx := context.Background()

	for _, obj := range []interface{}{&Product{}, &Order{}, &OrderItem{}} {
		_ = crud.DropTable(ctx, obj)
		if err := crud.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed: %s", err.Error())
		}
	}

	keyboard := &Product{Name: "Keyboard", Price: 4999, Stock: 5}
	mouse := &Product{Name: "Mouse", Price: 1999, Stock: 1}
	for _, product := range []*Product{keyboard, mouse} {
		if err := crud.Save(ctx, product, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	return keyboard, mouse
}

func fatalIfStock(t *testing.T, crud *CRUD, product *Product, want int64) {
	t.Helper()

	loaded := &Product{}
	if err := crud.Load(context.Background(), loaded, strconv.FormatUint(product.ID, 10), LoadOptions{}).Error; err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if loaded.Stock != want {
		t.Fatalf("%s has stock %d, want %d", loaded.Name, loaded.Stock, want)
	}
}

func fatalIfCount(t *testing.T, crud *CRUD, obj interface{}, want uint64) {
	t.Helper()

	cnt, err := crud.GetCount(context.Background(), obj, GetCountOptions{})
	if err != nil {
		t.Fatalf("GetCount failed: %s", err.Error())
	}
	if cnt != want {
		t.Fatalf("GetCount returned %d for %T, want %d", cnt, obj, want)
	}
}

func testRunInTxCommit(t *testing.T, crud *CRUD) {
	keyboard, mouse := createShopTables(t, crud)

	order, err := placeOrder(context.Background(), crud, 7, []CartItem{
		{ProductID: keyboard.ID, Quantity: 2},
		{ProductID: mouse.ID, Quantity: 1},
	})
	if err != nil {
		t.Fatalf("placeOrder failed: %s", err.Error())
	}
	if order.Total != 2*4999+1999 {
		t.Fatalf("placeOrder returned total %d, want %d", order.Total, 2*4999+1999)
	}

	loaded := &Order{}
	if err := crud.Load(context.Background(), loaded, strconv.FormatUint(order.ID, 10), LoadOptions{}).Error; err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if *loaded != *order {
		t.Fatalf("Load returned %+v, want %+v", *loaded, *order)
	}

	fatalIfCount(t, crud, &OrderItem{}, 2)
	fatalIfStock(t, crud, keyboard, 3)
	fatalIfStock(t, crud, mouse, 0)
}

func testRunInTxRollback(t *testing.T, crud *CRUD) {
	keyboard, mouse := createShopTables(t, crud)

	_, err := placeOrder(context.Background(), crud, 7, []CartItem{
		{ProductID: keyboard.ID, Quantity: 2},
		{ProductID: mouse.ID, Quantity: 3},
	})
	if !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("placeOrder returned err %v, want %v", err, ErrOutOfStock)
	}

	fatalIfCount(t, crud, &Order{}, 0)
	fatalIfCount(t, crud, &OrderItem{}, 0)
	fatalIfStock(t, crud, keyboard, 5)
	fatalIfStock(t, crud, mouse, 1)

	_, err = placeOrder(context.Background(), crud, 7, []CartItem{{ProductID: 999, Quantity: 1}})
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("placeOrder returned err %v, want %v", err, ErrProductNotFound)
	}
	fatalIfCount(t, crud, &Order{}, 0)
}

func testRunInTxRollbackOnValidationError(t *testing.T, crud *CRUD) {
	createShopTables(t, crud)

	err := crud.RunInTx(context.Background(), func(tx *CRUD) error {
		if err := tx.Save(context.Background(), &Product{Name: "Monitor", Stock: 2}, SaveOptions{}); err != nil {
			return err
		}
		return tx.Save(context.Background(), &Product{Name: "", Stock: 2}, SaveOptions{})
	})

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Violations["Name"] != "required" {
		t.Fatalf("RunInTx returned err %v, want a validation error for Name", err)
	}
	fatalIfCount(t, crud, &Product{}, 2)
}

func testRunInTxRollbackOnPanic(t *testing.T, crud *CRUD) {
	createShopTables(t, crud)

	func() {
		defer func() {
			if p := recover(); p != "boom" {
				t.Fatalf("RunInTx recovered %v, want the panic to be passed on", p)
			}
		}()

		_ = crud.RunInTx(context.Background(), func(tx *CRUD) error {
			_ = tx.Save(context.Background(), &Product{Name: "Monitor"}, SaveOptions{})
			panic("boom")
		})
	}()

	fatalIfCount(t, crud, &Product{}, 2)
}

func testRunInTxNested(t *testing.T, crud *CRUD) {
	createShopTables(t, crud)
	errAbort := errors.New("abort")

	err := crud.RunInTx(context.Background(), func(tx *CRUD) error {
		err := tx.RunInTx(context.Background(), func(nestedTx *CRUD) error {
			if nestedTx.Tx() != tx.Tx() {
				t.Fatal("nested RunInTx started a new transaction")
			}
			return nestedTx.Save(context.Background(), &Product{Name: "Monitor"}, SaveOptions{})
		})
		if err != nil {
			return err
		}
		return errAbort
	})
	if !errors.Is(err, errAbort) {
		t.Fatalf("RunInTx returned err %v, want %v", err, errAbort)
	}

	fatalIfCount(t, crud, &Product{}, 2)
}

func testWithTx(t *testing.T, crud *CRUD, db *sql.DB) {
	createShopTables(t, crud)
	ctx := context.Background()

	for _, commit := range []bool{false, true} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx failed: %s", err.Error())
		}

		txCRUD := crud.WithTx(tx)
		if txCRUD.Tx() != tx || crud.Tx() != nil {
			t.Fatal("WithTx did not return a CRUD running in the transaction only")
		}

		if err := txCRUD.Save(ctx, &Product{Name: "Monitor"}, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
		if _, err := tx.ExecContext(ctx, `UPDATE product SET stock = 0`); err != nil {
			t.Fatalf("ExecContext failed: %s", err.Error())
		}

		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatalf("finishing the transaction failed: %s", err.Error())
		}
	}

	fatalIfCount(t, crud, &Product{}, 3)
	cnt, _ := crud.GetCount(ctx, &Product{}, GetCountOptions{
		Filters: &sqlfilters.Filters{"Stock": {Op: sqlfilters.OpEqual, Val: 0}},
	})
	if cnt != 3 {
		t.Fatalf("committed transaction updated %d products, want 3", cnt)
	}
}

func testRunInTxGetCountOnUniq(t *testing.T, db *sql.DB, dialect string) {
	crud := New(db, Options{Dialect: dialect, Flags: GetCountOnUniq})
	_ = crud.DropTable(context.Background(), &test.TestStruct{})
	_ = crud.CreateTable(context.Background(), &test.TestStruct{})

	err := crud.RunInTx(context.Background(), func(tx *CRUD) error {
		if err := tx.Save(context.Background(), test.TestStructWithData(), SaveOptions{}); err != nil {
			return err
		}

		duplicate := test.TestStructWithData()
		duplicate.Key = "123456789012345678901234567890duplicate"
		if err := tx.Save(context.Background(), duplicate, SaveOptions{}); err != nil {
			return err
		}

		duplicate.ID = 0
		return tx.Save(context.Background(), duplicate, SaveOptions{})
	})

	var uniqErr *UniqError
	if !errors.As(err, &uniqErr) || uniqErr.Field != "Key" {
		t.Fatalf("RunInTx returned err %v, want an uniq error for Key", err)
	}
	fatalIfCount(t, crud, &test.TestStruct{}, 0)
}

func TestRunInTx_Commit(t *testing.T)        { testRunInTxCommit(t, testCRUD) }
func TestSQLite_RunInTx_Commit(t *testing.T) { testRunInTxCommit(t, testCRUDSQLite) }

func TestRunInTx_Rollback(t *testing.T)        { testRunInTxRollback(t, testCRUD) }
func TestSQLite_RunInTx_Rollback(t *testing.T) { testRunInTxRollback(t, testCRUDSQLite) }

func TestRunInTx_RollbackOnValidationError(t *testing.T) {
	testRunInTxRollbackOnValidationError(t, testCRUD)
}
func TestSQLite_RunInTx_RollbackOnValidationError(t *testing.T) {
	testRunInTxRollbackOnValidationError(t, testCRUDSQLite)
}

func TestRunInTx_RollbackOnPanic(t *testing.T)        { testRunInTxRollbackOnPanic(t, testCRUD) }
func TestSQLite_RunInTx_RollbackOnPanic(t *testing.T) { testRunInTxRollbackOnPanic(t, testCRUDSQLite) }

func TestRunInTx_Nested(t *testing.T)        { testRunInTxNested(t, testCRUD) }
func TestSQLite_RunInTx_Nested(t *testing.T) { testRunInTxNested(t, testCRUDSQLite) }

func TestWithTx(t *testing.T)        { testWithTx(t, testCRUD, testDB) }
func TestSQLite_WithTx(t *testing.T) { testWithTx(t, testCRUDSQLite, testDBSQLite) }

func TestRunInTx_GetCountOnUniq(t *testing.T) {
	testRunInTxGetCountOnUniq(t, testDB, DialectPostgres)
}
func TestSQLite_RunInTx_GetCountOnUniq(t *testing.T) {
	testRunInTxGetCountOnUniq(t, testDBSQLite, DialectSQLite)
}

func TestRunInTx_Isolation(t *testing.T) {
	createShopTables(t, testCRUD)

	err := testCRUD.RunInTx(context.Background(), func(tx *CRUD) error {
		if err := tx.Save(context.Background(), &Product{Name: "Monitor"}, SaveOptions{}); err != nil {
			return err
		}

		fatalIfCount(t, tx, &Product{}, 3)
		fatalIfCount(t, testCRUD, &Product{}, 2)
		return nil
	})
	if err != nil {
		t.Fatalf("RunInTx failed: %s", err.Error())
	}

	fatalIfCount(t, testCRUD, &Product{}, 3)
}

func TestRunInTxWithOptions_ReadOnly(t *testing.T) {
	createShopTables(t, testCRUD)

	err := testCRUD.RunInTxWithOptions(context.Background(), &sql.TxOptions{ReadOnly: true}, func(tx *CRUD) error {
		return tx.Save(context.Background(), &Product{Name: "Monitor"}, SaveOptions{})
	})
	if err == nil {
		t.Fatal("Save in a read-only transaction did not return an err")
	}
	fatalIfCount(t, testCRUD, &Product{}, 2)
}
