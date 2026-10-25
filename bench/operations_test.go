package bench

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type Customer struct {
	ID      uint64
	Name    string
	Country string
}

type Product struct {
	ID         uint64
	CustomerID uint64
	Name       string
	Category   string `crud:"idx"`
	Price      int64
	Stock      int64
	Active     bool
	AddedAt    time.Time
	Customer   *Customer `crud:"rel:CustomerID"`
}

// tableNamePrefix keeps the tables of the benchmarks apart from other tables of the database.
const tableNamePrefix = "bench_"

const selectProducts = `SELECT "id","customer_id","name","category","price","stock","active","added_at" FROM "bench_product"`

var ctx = context.Background()

// env is a database of the benchmarks, with a CRUD of its dialect.
type env struct {
	db     *sql.DB
	sqlite bool
	crud   *gocrud.CRUD
}

func newEnv(db *sql.DB, dialect string) *env {
	e := &env{db: db, sqlite: dialect == gocrud.DialectSQLite}
	e.crud = newCRUD(e)
	return e
}

func newCRUD(e *env) *gocrud.CRUD {
	dialect := gocrud.DialectPostgres
	if e.sqlite {
		dialect = gocrud.DialectSQLite
	}
	return gocrud.New(e.db, gocrud.Options{Dialect: dialect, TableNamePrefix: tableNamePrefix})
}

var regexpPlaceholder = regexp.MustCompile(`\$\d+`)

// q returns a query written with the placeholders of PostgreSQL in the placeholders of the dialect.
func (e *env) q(query string) string {
	if e.sqlite {
		return regexpPlaceholder.ReplaceAllString(query, "?")
	}
	return query
}

// operation is an operation done with gocrud, and with the same queries written with database/sql.
type operation struct {
	gocrud func() error
	sql    func() error
}

// operations are the operations of the benchmarks. On the fake database, a SELECT returns fakeRows rows. The real
// databases have the data of seed, in which each operation finds the same rows.
var operations = []struct {
	name      string
	fakeRows  int
	operation func(e *env) operation
}{
	{"op=Insert", 1, insert},
	{"op=Update", 1, update},
	{"op=Load", 1, load},
	{"op=LoadFirst", 1, loadFirst},
	{"op=Get/rows=1", 1, get(1)},
	{"op=Get/rows=10", 10, get(10)},
	{"op=Get/rows=100", 100, get(100)},
	{"op=Get/rows=1000", 1000, get(1000)},
	{"op=GetFilters/filter=fields", 11, getFilters(11, filterQuery{
		filter: sqlfilters.Filters{
			"Active":   {Op: sqlfilters.OpEqual, Val: true},
			"Category": {Op: sqlfilters.OpIn, Val: []string{"c1", "c10"}},
			"Price":    {Op: sqlfilters.OpGreaterOrEqual, Val: 100},
		},
		where: `"active"=$1 AND "category" IN ($2,$3) AND "price">=$4`, args: []any{true, "c1", "c10", 100},
	})},
	{"op=GetFilters/filter=or", 10, getFilters(10, filterQuery{
		filter: sqlfilters.Or(
			sqlfilters.Filters{"Category": {Op: sqlfilters.OpEqual, Val: "c10"}},
			sqlfilters.Filters{"Price": {Op: sqlfilters.OpLower, Val: 100}, "Stock": {Op: sqlfilters.OpGreater, Val: 0}},
		),
		where: `"category"=$1 OR ("price"<$2 AND "stock">$3)`, args: []any{"c10", 100, 0},
	})},
	{"op=GetFilters/filter=path", 5, getFilters(5, filterQuery{
		filter: sqlfilters.Filters{
			"Category":         {Op: sqlfilters.OpEqual, Val: "c10"},
			"Customer.Country": {Op: sqlfilters.OpEqual, Val: "PL"},
		},
		where: `"category"=$1 AND EXISTS (SELECT 1 FROM "bench_customer" r1 WHERE r1."id" = "bench_product"."customer_id" AND r1."country"=$2)`,
		args:  []any{"c10", "PL"},
	})},
	{"op=GetFilters/filter=subquery", 5, getFilters(5, filterQuery{
		filter: sqlfilters.Filters{
			"Category":   {Op: sqlfilters.OpEqual, Val: "c10"},
			"CustomerID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&Customer{}, "ID", sqlfilters.Filters{"Country": {Op: sqlfilters.OpEqual, Val: "PL"}})},
		},
		where: `"category"=$1 AND "customer_id" IN (SELECT s1."id" FROM "bench_customer" s1 WHERE s1."country"=$2)`,
		args:  []any{"c10", "PL"},
	})},
	{"op=GetCount", 10, getCount},
	{"op=UpdateMultiple", 10, updateMultiple},
	{"op=DeleteMultiple", 10, deleteMultiple},
}

// runOperations runs a benchmark of each operation with gocrud and with database/sql, as lib=gocrud and lib=sql, so
// that benchstat -col /lib shows them side by side.
func runOperations(b *testing.B, newEnv func(fakeRows int) *env) {
	for _, o := range operations {
		b.Run(o.name, func(b *testing.B) {
			op := o.operation(newEnv(o.fakeRows))

			for _, lib := range []struct {
				name string
				run  func() error
			}{{"gocrud", op.gocrud}, {"sql", op.sql}} {
				b.Run("lib="+lib.name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if err := lib.run(); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// seed creates the tables of the benchmarks with: 10 customers, every other one from PL; the product with ID 1 in the
// category new, which Update changes; and 1, 10, 100 and 1000 products in the categories c1, c10, c100 and c1000.
func seed(e *env) error {
	for _, obj := range []any{&Product{}, &Customer{}} {
		if err := e.crud.DropTable(ctx, obj); err != nil {
			return err
		}
	}
	for _, obj := range []any{&Customer{}, &Product{}} {
		if err := e.crud.CreateTable(ctx, obj); err != nil {
			return err
		}
	}

	return e.crud.RunInTx(ctx, func(tx *gocrud.CRUD) error {
		for i := 1; i <= 10; i++ {
			country := "DE"
			if i%2 == 1 {
				country = "PL"
			}
			if err := tx.Save(ctx, &Customer{Name: fmt.Sprintf("Customer %d", i), Country: country}, gocrud.SaveOptions{}); err != nil {
				return err
			}
		}

		if err := tx.Save(ctx, newProduct(), gocrud.SaveOptions{}); err != nil {
			return err
		}
		for _, n := range []int{1, 10, 100, 1000} {
			for i := range n {
				p := newProduct()
				p.CustomerID = uint64(i%10 + 1)
				p.Category = fmt.Sprintf("c%d", n)
				if err := tx.Save(ctx, p, gocrud.SaveOptions{}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func newProduct() *Product {
	return &Product{CustomerID: 1, Name: "Product", Category: "new", Price: 1000, Stock: 5, Active: true, AddedAt: time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)}
}

func scanProduct(row interface{ Scan(...any) error }, p *Product) error {
	return row.Scan(&p.ID, &p.CustomerID, &p.Name, &p.Category, &p.Price, &p.Stock, &p.Active, &p.AddedAt)
}

func insert(e *env) operation {
	return operation{
		gocrud: func() error {
			return e.crud.Save(ctx, newProduct(), gocrud.SaveOptions{})
		},
		sql: func() error {
			p := newProduct()
			return e.db.QueryRowContext(ctx, e.q(`INSERT INTO "bench_product"("customer_id","name","category","price","stock","active","added_at") VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING "id"`),
				p.CustomerID, p.Name, p.Category, p.Price, p.Stock, p.Active, e.crud.TimeValue(p.AddedAt)).Scan(&p.ID)
		},
	}
}

func update(e *env) operation {
	p := newProduct()
	p.ID = 1

	return operation{
		gocrud: func() error {
			return e.crud.Save(ctx, p, gocrud.SaveOptions{NoInsert: true})
		},
		sql: func() error {
			result, err := e.db.ExecContext(ctx, e.q(`UPDATE "bench_product" SET "customer_id"=$1,"name"=$2,"category"=$3,"price"=$4,"stock"=$5,"active"=$6,"added_at"=$7 WHERE "id" = $8`),
				p.CustomerID, p.Name, p.Category, p.Price, p.Stock, p.Active, e.crud.TimeValue(p.AddedAt), p.ID)
			return wantUpdated(result, err)
		},
	}
}

func wantUpdated(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n == 0 {
		return errors.New("not updated")
	}
	return nil
}

func loadProduct(e *env) error {
	var p Product
	return scanProduct(e.db.QueryRowContext(ctx, e.q(selectProducts+` WHERE "id" = $1`), uint64(1)), &p)
}

func load(e *env) operation {
	return operation{
		gocrud: func() error {
			_, err := gocrud.Load[Product](ctx, e.crud, uint64(1))
			return err
		},
		sql: func() error { return loadProduct(e) },
	}
}

// loadFirst is the first Load of a struct with a new CRUD, which reflects the struct and builds its queries.
// database/sql does not reflect structs, so its first Load is the same as the others.
func loadFirst(e *env) operation {
	return operation{
		gocrud: func() error {
			_, err := gocrud.Load[Product](ctx, newCRUD(e), uint64(1))
			return err
		},
		sql: func() error { return loadProduct(e) },
	}
}

func getProducts(e *env, query string, args ...any) ([]*Product, error) {
	rows, err := e.db.QueryContext(ctx, e.q(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var products []*Product
	for rows.Next() {
		p := &Product{}
		if err := scanProduct(rows, p); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func wantRows[T any](n int, rows []T, err error) error {
	if err == nil && len(rows) != n {
		return fmt.Errorf("got %d rows, want %d", len(rows), n)
	}
	return err
}

func get(n int) func(e *env) operation {
	category := fmt.Sprintf("c%d", n)
	query := selectProducts + ` WHERE "category"=$1 ORDER BY "id" ASC LIMIT 1000`

	return func(e *env) operation {
		return operation{
			gocrud: func() error {
				products, err := gocrud.Get[Product](ctx, e.crud, gocrud.GetOptions{
					Filters: sqlfilters.Filters{"Category": {Op: sqlfilters.OpEqual, Val: category}},
					Order:   []string{"ID", "asc"},
					Limit:   1000,
				})
				return wantRows(n, products, err)
			},
			sql: func() error {
				products, err := getProducts(e, query, category)
				return wantRows(n, products, err)
			},
		}
	}
}

// filterQuery is a filter of gocrud, and the WHERE clause and values of the same filter written by hand.
type filterQuery struct {
	filter sqlfilters.Filter
	where  string
	args   []any
}

// getFilters is Get of n rows with a filter.
func getFilters(n int, f filterQuery) func(e *env) operation {
	return func(e *env) operation {
		return operation{
			gocrud: func() error {
				products, err := gocrud.Get[Product](ctx, e.crud, gocrud.GetOptions{Filters: f.filter})
				return wantRows(n, products, err)
			},
			sql: func() error {
				products, err := getProducts(e, selectProducts+` WHERE `+f.where, f.args...)
				return wantRows(n, products, err)
			},
		}
	}
}

func getCount(e *env) operation {
	query := `SELECT COUNT(*) AS cnt FROM "bench_product" WHERE "category"=$1`

	return operation{
		gocrud: func() error {
			_, err := gocrud.GetCount[Product](ctx, e.crud, gocrud.GetCountOptions{
				Filters: sqlfilters.Filters{"Category": {Op: sqlfilters.OpEqual, Val: "c10"}},
			})
			return err
		},
		sql: func() error {
			var count uint64
			return e.db.QueryRowContext(ctx, e.q(query), "c10").Scan(&count)
		},
	}
}

func updateMultiple(e *env) operation {
	query := `UPDATE "bench_product" SET "stock"=$1 WHERE "category"=$2`

	return operation{
		gocrud: func() error {
			return gocrud.UpdateMultiple[Product](ctx, e.crud, map[string]any{"Stock": int64(0)}, gocrud.UpdateMultipleOptions{
				Filters: sqlfilters.Filters{"Category": {Op: sqlfilters.OpEqual, Val: "c10"}},
			})
		},
		sql: func() error {
			_, err := e.db.ExecContext(ctx, e.q(query), int64(0), "c10")
			return err
		},
	}
}

var errRollback = errors.New("rollback")

// rolledBack returns nil for the error of a transaction that was rolled back on purpose.
func rolledBack(err error) error {
	if errors.Is(err, errRollback) {
		return nil
	}
	return err
}

// deleteMultiple deletes the products in a transaction that is rolled back, so that each run deletes the same rows.
func deleteMultiple(e *env) operation {
	query := `DELETE FROM "bench_product" WHERE "category"=$1 RETURNING "id"`

	return operation{
		gocrud: func() error {
			return rolledBack(e.crud.RunInTx(ctx, func(tx *gocrud.CRUD) error {
				ids, err := gocrud.DeleteMultiple[Product](ctx, tx, gocrud.DeleteMultipleOptions{
					Filters: sqlfilters.Filters{"Category": {Op: sqlfilters.OpEqual, Val: "c10"}},
				})
				if err := wantRows(10, ids, err); err != nil {
					return err
				}
				return errRollback
			}))
		},
		sql: func() error {
			tx, err := e.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback() }()

			rows, err := tx.QueryContext(ctx, e.q(query), "c10")
			if err != nil {
				return err
			}
			defer func() { _ = rows.Close() }()

			var ids []uint64
			for rows.Next() {
				var id uint64
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return wantRows(10, ids, rows.Err())
		},
	}
}
