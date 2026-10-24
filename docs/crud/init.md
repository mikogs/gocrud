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
    Flags           uint64
    QueryHook       QueryHook
}
```

* **TableNamePrefix (string)**: Allows you to define a prefix for all database table names managed by this instance. This is useful for organizing tables in shared databases or separating environments (e.g., dev_, prod_). If left empty, the library uses the default table name derived from the struct. The prefix can have only lowercase letters, digits and underscores, as it is a part of the SQL of the queries, otherwise `New` panics.
* **TagName (string)**: Specifies the name of the struct tag used for database mapping rules. Default: `crud`. If you set this to `mytag`, the library will look for tags like `mytag:"uniq"` instead of the default `crud:"uniq"`, and for regular expressions in `mytag_regexp` instead of `crud_regexp`. Validation rules are always read from the `validate` tag.
* **Dialect (string)** *(required)*: Selects the SQL dialect used to generate queries. Passing an empty or unrecognised value causes a panic at startup. Two constants are provided:
    * `gocrud.DialectPostgres` — PostgreSQL.
    * `gocrud.DialectSQLite` — SQLite. Use this when connecting via `modernc.org/sqlite`. Column types are mapped to SQLite affinities (`INTEGER`, `TEXT`, `REAL`) and `?` placeholders are used instead of `$1`, `$2`, …
* **Flags (uint64)**: Enables specific behaviors via bitwise flags. Currently, one flag is supported: **GetCountOnUniq (const GetCountOnUniq = 1)**: When this flag is set, the library performs an additional COUNT(*) query on unique fields before attempting an insert or update. This ensures that the unique value does not already exist in the database, providing an extra layer of validation beyond standard database constraints. A flag can also be set later, with `crudInstance.SetFlag(gocrud.GetCountOnUniq)`.
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
    Flags:           gocrud.GetCountOnUniq,
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

`QueryHook` is called around each query that gocrud runs, also in transactions and migrations, e.g. to log the queries, to find the slow ones, or to trace them:

```go
type QueryHook interface {
    BeforeQuery(ctx context.Context, event *QueryEvent) context.Context
    AfterQuery(ctx context.Context, event *QueryEvent)
}

type QueryEvent struct {
    Query    string
    Args     []interface{}
    Duration time.Duration // set for AfterQuery
    Err      error         // set for AfterQuery
}
```

`BeforeQuery` returns the context that the query runs with, which is also passed to `AfterQuery`, e.g. with the span of a trace that `AfterQuery` ends. `AfterQuery` is called when the database has run the query. For a query that returns rows, that is when the database starts returning them, before they are read, so `Err` does not have the errors that the driver returns only when the rows are read, e.g. some errors of SQLite. A row that is not found, e.g. by `Load`, is not an error of the query.

For a hook that only needs `AfterQuery`, e.g. a log of the slow queries, `QueryHookFunc` is enough:

```go
crudInstance := gocrud.New(db, gocrud.Options{
    Dialect: gocrud.DialectPostgres,
    QueryHook: gocrud.QueryHookFunc(func(ctx context.Context, event *gocrud.QueryEvent) {
        if event.Err != nil || event.Duration > 100*time.Millisecond {
            slog.WarnContext(ctx, "query", "sql", event.Query, "duration", event.Duration, "err", event.Err)
        }
    }),
})
```

`Args` are the values of the placeholders, as `database/sql` converts them for the driver, e.g. the value of the `Value` method of a [Valuer type](../struct.md#7-valuer-types). They include the bcrypt hashes of the passwords that are saved, and any personal data of the objects, so log them with care. Without a hook, the queries are run on the database directly, with no overhead.

The hook is called only for the queries of gocrud, not for `BEGIN`, `COMMIT` and `ROLLBACK` of transactions, or the queries that you run yourself, e.g. on `tx.Tx()`.
