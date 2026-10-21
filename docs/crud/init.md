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
}
```

* **TableNamePrefix (string)**: Allows you to define a prefix for all database table names managed by this instance. This is useful for organizing tables in shared databases or separating environments (e.g., dev_, prod_). If left empty, the library uses the default table name derived from the struct.
* **TagName (string)**: Specifies the name of the struct tag used for database mapping rules. Default: `crud`. If you set this to `mytag`, the library will look for tags like `mytag:"uniq"` instead of the default `crud:"uniq"`, and for regular expressions in `mytag_regexp` instead of `crud_regexp`. Validation rules are always read from the `validate` tag.
* **Dialect (string)** *(required)*: Selects the SQL dialect used to generate queries. Passing an empty or unrecognised value causes a panic at startup. Two constants are provided:
    * `gocrud.DialectPostgres` — PostgreSQL.
    * `gocrud.DialectSQLite` — SQLite. Use this when connecting via `modernc.org/sqlite`. Column types are mapped to SQLite affinities (`INTEGER`, `TEXT`, `REAL`) and `?` placeholders are used instead of `$1`, `$2`, …
* **Flags (uint64)**: Enables specific behaviors via bitwise flags. Currently, one flag is supported: **GetCountOnUniq (const GetCountOnUniq = 1)**: When this flag is set, the library performs an additional COUNT(*) query on unique fields before attempting an insert or update. This ensures that the unique value does not already exist in the database, providing an extra layer of validation beyond standard database constraints.

## Example Usage

### PostgreSQL

```go
import (
    "github.com/mikogs/gocrud"
    _ "github.com/lib/pq"
)

db, err := sql.Open("postgres", "host=localhost user=myuser password=mypass dbname=mydb sslmode=disable")

crudInstance := gocrud.New(db, gocrud.Options{
    Dialect:         gocrud.DialectPostgres,
    TableNamePrefix: "app_",
    TagName:         "crud",
    Flags:           gocrud.GetCountOnUniq,
})
```

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

