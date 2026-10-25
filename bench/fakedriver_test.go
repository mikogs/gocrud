package bench

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// fakeDriver is a database that answers every query at once, so that the benchmarks measure only the code that
// builds the queries and reads their rows. The name of a database is the number of rows that a SELECT returns.
type fakeDriver struct{}

func init() {
	sql.Register("fake", fakeDriver{})
}

var (
	fakeDBsMu sync.Mutex
	// fakeDBs are the fake databases by the number of rows, which TestMain closes
	fakeDBs = map[int]*sql.DB{}
)

// openFake returns a database whose SELECT queries return rows products.
func openFake(rows int) *sql.DB {
	fakeDBsMu.Lock()
	defer fakeDBsMu.Unlock()

	if db, ok := fakeDBs[rows]; ok {
		return db
	}
	db, err := sql.Open("fake", strconv.Itoa(rows))
	if err != nil {
		panic(err)
	}
	fakeDBs[rows] = db
	return db
}

var productColumns = []string{"id", "customer_id", "name", "category", "price", "stock", "active", "added_at"}

func (fakeDriver) Open(name string) (driver.Conn, error) {
	n, err := strconv.Atoi(name)
	if err != nil {
		return nil, err
	}

	added := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	conn := &fakeConn{products: make([][]driver.Value, n)}
	for i := range conn.products {
		conn.products[i] = []driver.Value{int64(i + 1), int64(1), fmt.Sprintf("Product %d", i+1), "books", int64(1000), int64(5), true, added}
	}
	return conn, nil
}

type fakeConn struct {
	products [][]driver.Value
}

// recordQuery is called with each query and its arguments when it is set.
var recordQuery func(query string, args []driver.NamedValue)

func (c *fakeConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if recordQuery != nil {
		recordQuery(query, args)
	}

	upper := strings.ToUpper(query)
	switch {
	case strings.HasPrefix(upper, "INSERT"):
		return &fakeRows{columns: []string{"id"}, values: [][]driver.Value{{int64(1)}}}, nil
	case strings.HasPrefix(upper, "SELECT COUNT"):
		return &fakeRows{columns: []string{"cnt"}, values: [][]driver.Value{{int64(len(c.products))}}}, nil
	case strings.HasPrefix(upper, "SELECT"):
		return &fakeRows{columns: productColumns, values: c.products}, nil
	case strings.HasPrefix(upper, "DELETE"):
		ids := make([][]driver.Value, len(c.products))
		for i, product := range c.products {
			ids[i] = product[:1]
		}
		return &fakeRows{columns: []string{"id"}, values: ids}, nil
	}
	return nil, fmt.Errorf("fake: unexpected query %q", query)
}

func (c *fakeConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if recordQuery != nil {
		recordQuery(query, args)
	}
	return driver.RowsAffected(1), nil
}

func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("fake: queries are not prepared")
}

func (c *fakeConn) Close() error { return nil }

func (c *fakeConn) Begin() (driver.Tx, error) { return fakeTx{}, nil }

type fakeTx struct{}

func (fakeTx) Commit() error   { return nil }
func (fakeTx) Rollback() error { return nil }

type fakeRows struct {
	columns []string
	values  [][]driver.Value
	next    int
}

func (r *fakeRows) Columns() []string { return r.columns }

func (r *fakeRows) Close() error { return nil }

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.next == len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.next])
	r.next++
	return nil
}
