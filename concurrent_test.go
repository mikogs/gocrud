package gocrud

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type ConcCustomer struct {
	ID      uint64
	Country string
	Orders  []ConcOrder `crud:"rel:CustomerID"`
}

type ConcOrder struct {
	ID         uint64
	CustomerID uint64
	Worker     int64
	Total      int64
	Customer   *ConcCustomer `crud:"rel:CustomerID"`
}

type concTotals struct {
	Country string `crud:"group:Customer.Country"`
	Orders  int64  `crud:"agg:count"`
}

// testConcurrentUse runs the operations of a new CRUD, whose caches are empty, on goroutines at the same time, so
// that the race detector can find the data races of the code that they share.
func testConcurrentUse(t *testing.T, db *sql.DB, dialect string) {
	ctx := context.Background()
	setup := New(db, Options{Dialect: dialect})
	for _, obj := range []any{&ConcOrder{}, &ConcCustomer{}} {
		_ = setup.DropTable(ctx, obj)
	}
	for _, obj := range []any{&ConcCustomer{}, &ConcOrder{}} {
		if err := setup.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed: %s", err.Error())
		}
	}
	if err := setup.Save(ctx, &ConcCustomer{Country: "PL"}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	crud := New(db, Options{Dialect: dialect})
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			for i := range 10 {
				if err := concurrentOperations(ctx, crud, int64(worker)); err != nil {
					t.Errorf("worker %d, iteration %d: %s", worker, i, err.Error())
					return
				}
			}
		})
	}
	wg.Wait()
}

// concurrentOperations does each operation once, on the orders of the worker.
func concurrentOperations(ctx context.Context, crud *CRUD, worker int64) error {
	order := &ConcOrder{CustomerID: 1, Worker: worker, Total: 100}
	if err := crud.Save(ctx, order, SaveOptions{}); err != nil {
		return fmt.Errorf("Save: %w", err)
	}
	if _, err := Load[ConcOrder](ctx, crud, order.ID, "Customer"); err != nil {
		return fmt.Errorf("Load: %w", err)
	}

	ofWorker := sqlfilters.Filters{
		"Worker":           {Op: sqlfilters.OpEqual, Val: worker},
		"Customer.Country": {Op: sqlfilters.OpEqual, Val: "PL"},
		"CustomerID":       {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&ConcCustomer{}, "ID", sqlfilters.Filters{"Country": {Op: sqlfilters.OpEqual, Val: "PL"}})},
	}
	if orders, err := Get[ConcOrder](ctx, crud, GetOptions{Filters: ofWorker, Order: []string{"Customer.Country", "asc"}, Preload: []string{"Customer"}}); err != nil || len(orders) != 1 {
		return fmt.Errorf("Get returned %d orders, %w, want 1", len(orders), err)
	}
	if _, err := GetCount[ConcCustomer](ctx, crud, GetCountOptions{Filters: sqlfilters.Filters{"Orders.Worker": {Op: sqlfilters.OpEqual, Val: worker}}}); err != nil {
		return fmt.Errorf("GetCount: %w", err)
	}
	if err := UpdateMultiple[ConcOrder](ctx, crud, map[string]any{"Total": int64(200)}, UpdateMultipleOptions{Filters: ofWorker}); err != nil {
		return fmt.Errorf("UpdateMultiple: %w", err)
	}
	if _, err := Aggregate[ConcOrder, concTotals](ctx, crud, AggregateOptions{Filters: ofWorker}); err != nil {
		return fmt.Errorf("Aggregate: %w", err)
	}

	err := crud.RunInTx(ctx, func(tx *CRUD) error {
		order.Total = 300
		if err := tx.Save(ctx, order, SaveOptions{NoInsert: true}); err != nil {
			return err
		}
		_, err := Load[ConcOrder](ctx, tx, order.ID)
		return err
	})
	if err != nil {
		return fmt.Errorf("RunInTx: %w", err)
	}

	if err := crud.Delete(ctx, order, DeleteOptions{}); err != nil {
		return fmt.Errorf("Delete: %w", err)
	}
	return nil
}

func TestConcurrentUse(t *testing.T) { testConcurrentUse(t, testDB, DialectPostgres) }

func TestSQLite_ConcurrentUse(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// a database in memory is a database of one connection
	db.SetMaxOpenConns(1)

	testConcurrentUse(t, db, DialectSQLite)
}
