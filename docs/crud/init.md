# Initialization

To begin using the CRUD library, you must initialize a new instance by passing your database connection and optional configuration settings. This creates the controller object that manages all subsequent database operations.

## Basic Initialization

Create a new CRUD instance using the `New` function:

```go
crudInstance := gocrud.New(dbConn, gocrud.Options{
    Dialect: gocrud.DialectPostgres,
})
```

`Dialect` is **required**. Passing an empty or unrecognised value causes a panic at startup. Use one of the two provided constants: `gocrud.DialectPostgres` or `gocrud.DialectSQLite`.

## Configuration Options

The `Options` struct allows you to customize the behavior of the CRUD instance during initialization:

```go
type Options struct {
    TableNamePrefix string
    TagName         string
    Dialect         string
    GetCountOnUniq  bool
    QueryHook       QueryHook
}
```

* **TableNamePrefix (string)**: Allows you to define a prefix for all database table names managed by this instance. This is useful for organizing tables in shared databases or separating environments (e.g., dev_, prod_). If left empty, the library uses the default table name derived from the struct. The prefix can have only lowercase letters, digits and underscores, as it is a part of the SQL of the queries, otherwise `New` panics.
* **TagName (string)**: Specifies the name of the struct tag used for database mapping rules. Default: `crud`. If you set this to `mytag`, the library will look for tags like `mytag:"uniq"` instead of the default `crud:"uniq"`, and for regular expressions in `mytag_regexp` instead of `crud_regexp`. Validation rules are always read from the `validate` tag.
* **Dialect (string)** *(required)*: Selects the SQL dialect used to generate queries. Passing an empty or unrecognised value causes a panic at startup. Two constants are provided:
    * `gocrud.DialectPostgres` — PostgreSQL.
    * `gocrud.DialectSQLite` — SQLite. Use this when connecting via `modernc.org/sqlite`. Column types are mapped to SQLite affinities (`INTEGER`, `TEXT`, `REAL`) and `?` placeholders are used instead of `$1`, `$2`, …
* **GetCountOnUniq (bool)**: Makes `Save` run an additional `COUNT(*)` query on each unique field before an insert or an update, and return a `UniqError` when another row already has the value, also when the table has no `UNIQUE` constraint, e.g. one created by hand without it.
* **QueryHook (QueryHook)**: Is called around each query, see [Query hook](#query-hook).

## Example Usage

### PostgreSQL

```go
import (
    "github.com/mikogs/gocrud"
    _ "github.com/lib/pq"
)

db, err := sql.Open("postgres", "host=localhost user=myuser password=mypass dbname=mydb sslmode=disable statement_timeout=10000")

crudInstance := gocrud.New(db, gocrud.Options{
    Dialect:         gocrud.DialectPostgres,
    TableNamePrefix: "app_",
    TagName:         "crud",
    GetCountOnUniq:  true,
})
```

`statement_timeout=10000` makes PostgreSQL cancel any query that takes longer than 10 seconds, e.g. one with filters from a request that the database cannot answer quickly. gocrud also cancels a query when the `ctx` of the operation is cancelled, e.g. with `context.WithTimeout`.

The pgx driver works in the same way, with its name in `sql.Open`:

```go
import (
    "github.com/mikogs/gocrud"
    _ "github.com/jackc/pgx/v5/stdlib"
)

db, err := sql.Open("pgx", "host=localhost user=myuser password=mypass dbname=mydb sslmode=disable statement_timeout=10000")
```

gocrud does not depend on a driver: it recognizes the errors of both, e.g. a [`UniqError`](ops.md#errors), by what they have in common. The only differences are of the drivers themselves:

* **Times.** PostgreSQL stores microseconds: lib/pq rounds the nanoseconds of a time, and pgx cuts them off.
* **Prepared statements.** pgx prepares each query once on each connection and reuses it. When the type of a column changes while the program runs, e.g. a table is dropped and created again with another type of a column, or altered by hand, each connection fails each query of the table once, e.g. with `cached plan must not change result type`, and then prepares it again. Restart the program after such a change, or add `default_query_exec_mode=describe_exec` to the connection string, so that pgx does not keep the statements, at the cost of a round trip to the database per query. `Migrate` only adds columns, which does not change the queries of gocrud, as they name their columns.

### SQLite

```go
import (
    "github.com/mikogs/gocrud"
    _ "modernc.org/sqlite"
)

db, err := sql.Open("sqlite", "./myapp.db")

crudInstance := gocrud.New(db, gocrud.Options{
    Dialect: gocrud.DialectSQLite,
})
```

Pass `:memory:` as the path to `sql.Open` for an in-memory SQLite database (useful for tests).

### Dialect differences

| Feature | PostgreSQL | SQLite |
|---|---|---|
| Placeholders | `$1`, `$2`, … | `?` |
| Auto-increment | `SERIAL PRIMARY KEY` | `INTEGER PRIMARY KEY` |
| String columns | `VARCHAR(255)` | `TEXT` |
| Integer columns | `BIGINT`, `INTEGER`, … | `INTEGER` |
| Boolean columns | `BOOLEAN` | `INTEGER` (0/1) |
| Float columns | `DOUBLE PRECISION` (`float64`), `REAL` (`float32`) | `REAL` |
| Time columns | `TIMESTAMPTZ` | `DATETIME` |
| Pointer fields | nullable column, without `NOT NULL` and a default | the same |
| Regex filter (`OpMatch`) | supported (`~`) | not supported |

## Query hook

`QueryHook` is called around each query that gocrud runs, also in transactions and migrations, e.g. to log the queries, to find the slow ones, to trace them, or to measure them:

```go
type QueryHook interface {
    BeforeQuery(ctx context.Context, event *QueryEvent) context.Context
    AfterQuery(ctx context.Context, event *QueryEvent)
}

type QueryEvent struct {
    Op       string // the operation, e.g. "Save", "Get", "Migrate", or "RunInTx"
    Table    string // the table of the struct, empty for "RunInTx"
    Query    string
    Args     []interface{}
    Duration time.Duration // set for AfterQuery
    Err      error         // set for AfterQuery
}
```

* **`BeforeQuery`** returns the context that the query runs with, which is also passed to `AfterQuery`, e.g. with the span of a trace that `AfterQuery` ends.
* **`AfterQuery`** is called when the database has run the query. For a query that returns rows, that is when the database starts returning them, before they are read, so `Err` does not have the errors that the driver returns only when the rows are read, e.g. some errors of SQLite. A row that is not found, e.g. by `Load`, is not an error of the query.
* **`Op` and `Table`** name the operation and its table, e.g. `"Get"` and `"order"`, also for the queries that an operation runs for another one, e.g. `"GetCount"` for the `GetCountOnUniq` check of `Save`, or `"Get"` and the related table for `Preload`. They have few values, so they can be the labels of metrics and the names of spans, unlike `Query`, which has the values of `IN` lists and of the limits.
* **Transactions** of `RunInTx` are events too: `BEGIN`, `COMMIT` and `ROLLBACK`, with the `Op` `"RunInTx"`. A transaction of `WithTx` is yours, so its statements are not.
* **`Args`** are the values of the placeholders, as `database/sql` converts them for the driver, e.g. the value of the `Value` method of a [Valuer type](../struct.md#7-valuer-types). They include the bcrypt hashes of the passwords that are saved, and any personal data of the objects, so log them with care.
* **Without a hook,** the queries are run on the database directly, with no overhead. The queries that you run yourself, e.g. on `tx.Tx()`, are not seen by the hook.

For a hook that only needs `AfterQuery`, e.g. a log of the slow queries, `QueryHookFunc` is enough:

```go
crudInstance := gocrud.New(db, gocrud.Options{
    Dialect: gocrud.DialectPostgres,
    QueryHook: gocrud.QueryHookFunc(func(ctx context.Context, event *gocrud.QueryEvent) {
        if event.Err != nil || event.Duration > 100*time.Millisecond {
            slog.WarnContext(ctx, "query", "op", event.Op, "table", event.Table, "sql", event.Query,
                "duration", event.Duration, "err", event.Err)
        }
    }),
})
```

### Metrics

gocrud does not depend on a library of metrics, but the hook gives everything that metrics need. E.g. with [Prometheus](https://github.com/prometheus/client_golang), a histogram of the durations of the queries, by the operation, the table and whether the query failed:

```go
queryDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
    Name:    "gocrud_query_duration_seconds",
    Help:    "The durations of the queries of gocrud.",
    Buckets: prometheus.DefBuckets,
}, []string{"op", "table", "status"})
prometheus.MustRegister(queryDuration)

crudInstance := gocrud.New(db, gocrud.Options{
    Dialect: gocrud.DialectPostgres,
    QueryHook: gocrud.QueryHookFunc(func(_ context.Context, event *gocrud.QueryEvent) {
        status := "ok"
        if event.Err != nil {
            status = "error"
        }
        queryDuration.WithLabelValues(event.Op, event.Table, status).Observe(event.Duration.Seconds())
    }),
})
```

Do not use `Query` or `Args` as labels: each different value is a new time series.

The connections of the database are often the first to run out, e.g. when queries wait for a free connection. `database/sql` counts them in `db.Stats()`, the open and the used connections, and how many times and how long the queries waited for one, and Prometheus collects them with `collectors.NewDBStatsCollector(db, "app")`, from `github.com/prometheus/client_golang/prometheus/collectors`.
