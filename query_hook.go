package gocrud

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"
)

// QueryHook is called around each query that gocrud runs, e.g. to log the queries, to find the slow ones, or to trace
// them. It is set in Options.QueryHook.
type QueryHook interface {
	// BeforeQuery is called before the query is sent. The context it returns is the context of the query, and of
	// AfterQuery, e.g. with the span of a trace.
	BeforeQuery(ctx context.Context, event *QueryEvent) context.Context
	// AfterQuery is called when the database has run the query, with Duration and Err of event set. For a query that
	// returns rows, it is called when the database starts returning them, before they are read, so an error that the
	// driver returns only when the rows are read is not in Err.
	AfterQuery(ctx context.Context, event *QueryEvent)
}

type QueryEvent struct {
	// Op is the operation that runs the query, e.g. "Save", "Get" or "Migrate", or "RunInTx" for BEGIN, COMMIT and
	// ROLLBACK, and Table is the table of its struct, or empty for a transaction. They name few values, e.g. for the
	// labels of metrics, unlike Query.
	Op    string
	Table string
	Query string
	// Args are the values of the placeholders, as database/sql converts them for the driver, e.g. the value of a
	// pointer to a field, or of the Value method of a Valuer. They include the hashes of the passwords that are saved.
	Args     []interface{}
	Duration time.Duration
	Err      error
}

// QueryHookFunc is a QueryHook of only AfterQuery, e.g. for a log of the queries.
type QueryHookFunc func(ctx context.Context, event *QueryEvent)

func (f QueryHookFunc) BeforeQuery(ctx context.Context, _ *QueryEvent) context.Context { return ctx }

func (f QueryHookFunc) AfterQuery(ctx context.Context, event *QueryEvent) { f(ctx, event) }

// hookedConn runs the queries of conn with a hook. It is used only when there is a hook, so that the queries without
// one have no overhead.
type hookedConn struct {
	conn DBTX
	hook QueryHook
}

func (h hookedConn) run(ctx context.Context, query string, args []interface{}, fn func(ctx context.Context) error) {
	runHooked(ctx, h.hook, query, args, fn)
}

// runHooked runs fn, the query, with the hook.
func runHooked(ctx context.Context, hook QueryHook, query string, args []interface{}, fn func(ctx context.Context) error) {
	info, _ := ctx.Value(queryInfoKey{}).(queryInfo)
	event := &QueryEvent{Op: info.op, Table: info.table, Query: query, Args: make([]interface{}, len(args))}
	for i, arg := range args {
		// the argument as it is when it cannot be converted, as the driver may convert it itself
		event.Args[i] = arg
		if value, err := driver.DefaultParameterConverter.ConvertValue(arg); err == nil {
			event.Args[i] = value
		}
	}
	ctx = hook.BeforeQuery(ctx, event)
	start := time.Now()
	event.Err = fn(ctx)
	event.Duration = time.Since(start)
	hook.AfterQuery(ctx, event)
}

type queryInfoKey struct{}

// queryInfo is the operation and the table of the queries of a context, for QueryEvent.
type queryInfo struct {
	op    string
	table string
}

// withQuery returns ctx with the operation and the table of its queries, when the CRUD has a hook.
func (c *CRUD) withQuery(ctx context.Context, op string, table string) context.Context {
	if c.hook == nil {
		return ctx
	}
	return context.WithValue(ctx, queryInfoKey{}, queryInfo{op: op, table: table})
}

// runTxStatement runs fn, which begins, commits or rolls back a transaction, with the hook of the CRUD, if it has one.
func (c *CRUD) runTxStatement(ctx context.Context, statement string, fn func(ctx context.Context) error) error {
	if c.hook == nil {
		return fn(ctx)
	}
	var err error
	runHooked(c.withQuery(ctx, "RunInTx", ""), c.hook, statement, nil, func(ctx context.Context) error {
		err = fn(ctx)
		return err
	})
	return err
}

func (h hookedConn) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	var result sql.Result
	var err error
	h.run(ctx, query, args, func(ctx context.Context) error {
		result, err = h.conn.ExecContext(ctx, query, args...)
		return err
	})
	return result, err
}

func (h hookedConn) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	var rows *sql.Rows
	var err error
	h.run(ctx, query, args, func(ctx context.Context) error {
		rows, err = h.conn.QueryContext(ctx, query, args...) //nolint:rowserrcheck,sqlclosecheck // the rows are returned, and their caller checks Err and closes them
		return err
	})
	return rows, err
}

func (h hookedConn) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	var row *sql.Row
	h.run(ctx, query, args, func(ctx context.Context) error {
		row = h.conn.QueryRowContext(ctx, query, args...)
		// the error of the query, as sql.ErrNoRows is returned only by Scan
		return row.Err()
	})
	return row
}

// hooked returns conn with the hook of the CRUD, if it has one.
func (c *CRUD) hooked(conn DBTX) DBTX {
	if c.hook == nil {
		return conn
	}
	return hookedConn{conn: conn, hook: c.hook}
}
