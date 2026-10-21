# Service

The `pkg/service` package provides a higher-level wrapper around the core CRUD object. Where the core layer works directly with struct instances, the service layer is driven by a **registry** — a map of string keys to constructor functions — and accepts filter values as plain strings, making it a natural fit for wiring up to HTTP handlers.

## Initialization

```go
import (
    "github.com/mikogs/gocrud"
    svccrud    "github.com/mikogs/gocrud/pkg/service"
)

svc := svccrud.New(
    map[string]func() interface{}{
        "users":    func() interface{} { return &User{} },
        "products": func() interface{} { return &Product{} },
    },
    dbConn,
    gocrud.DialectPostgres, // or gocrud.DialectSQLite
)
```

`New` takes three arguments:

* **registry** `map[string]func() interface{}` — maps string keys to constructor functions. Each function must return a pointer to a new zero-value struct.
* **dbConn** `*sql.DB` — an open database connection.
* **dialect** `string` — the SQL dialect; must be `gocrud.DialectPostgres` or `gocrud.DialectSQLite`. Passing an empty or unrecognised value causes a panic at startup.

### Creating tables

```go
err := svc.CreateTables(ctx)
```

Iterates over all registered keys and calls `CREATE TABLE IF NOT EXISTS` for each struct. Useful during application startup when `CREATE_TABLES=true` is set.

## Methods

### Save

```go
err := svc.Save(ctx, obj, userID)
```

Saves `obj` to the database. If `obj.ID == 0` an `INSERT` is performed; otherwise an upsert. `userID` is written to the `CreatedBy` and `ModifiedBy` fields, and the current time to `CreatedAt` and `ModifiedAt`, when the struct has them. The creation fields are never changed by an update, see [Creation and modification fields](../struct.md#4-creation-and-modification-fields).

**Returns** `*ModelValidationError` when struct validation fails, otherwise a generic error.

---

### Read

```go
obj, passwordFields, err := svc.Read(ctx, "users", id, nil, nil)
```

Loads a single record by `id`. Returns `NotFoundError` if no record with that ID exists.

The fourth argument is an optional constructor (`func() interface{}`). When `nil` the key's registered constructor is used. Pass a non-nil constructor to load the record into a different struct type — for example a read-specific projection with fewer fields, which will generate a narrower `SELECT`.

The fifth argument, `passFieldsToVerify`, maps a struct field name to a plaintext password to check against the stored hash. The returned `passwordFields` (`map[string]int`) holds `gocrud.PassOK` or `gocrud.PassInvalid` for each password field named in `passFieldsToVerify`; keys that do not name an actual password field are ignored.

```go
// The "_" suffix is stripped when deriving the table name:
//   strings.Split("User_Summary", "_")[0] = "User" → table "user"
type User_Summary struct {
    ID    uint64
    Email string
}

obj, _, err := svc.Read(ctx, "users", id, func() interface{} { return &User_Summary{} }, nil)
```

```go
// Verify a login password while loading the user record:
obj, passwordFields, err := svc.Read(ctx, "users", id, nil, map[string]string{
    "Password": submittedPassword,
})
if err == nil && passwordFields["Password"] == gocrud.PassOK {
    // password matches
}
```

---

### Delete

```go
err := svc.Delete(ctx, "users", id)
```

Reads the record by `id` first (returning `NotFoundError` if absent), then deletes it. Cascade rules defined on the struct are applied automatically.

---

### List

```go
objs, err := svc.List(
    ctx,
    "users",
    limit, offset,
    "Age", "asc",
    map[string]string{"Status": "1"},
    map[string]string{"Status": "eq"},
    nil, // optional row transform func
    nil, // optional constructor override
)
```

Returns a paginated, filtered, ordered list of objects for the given key. All filter values are passed as strings and converted to the correct field types automatically.

**Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `key` | `string` | Registry key |
| `limit` | `int` | SQL `LIMIT` |
| `offset` | `int` | SQL `OFFSET` |
| `order` | `string` | Field name to sort by (empty = no ordering) |
| `orderDirection` | `string` | `"asc"` or `"desc"` |
| `filterVals` | `map[string]string` | Field name → value |
| `filterOps` | `map[string]string` | Field name → operator string (see below) |
| `rowFunc` | `func(interface{}) interface{}` | Optional per-row transform; `nil` returns raw structs |
| `constructor` | `func() interface{}` | Optional struct constructor; `nil` uses the key's registered constructor. Pass a non-nil value to scan rows into a different struct type (e.g. a list projection with fewer fields). |

**Filter operators:**

| String | Meaning |
|---|---|
| `eq` | `=` (default when omitted) |
| `ne` | `!=` |
| `lt` | `<` |
| `le` | `<=` |
| `gt` | `>` |
| `ge` | `>=` |
| `like` | `LIKE` |
| `match` | `~` (regex, PostgreSQL only) |
| `bit` | bitwise AND `> 0` |

Values of `time.Time` fields can be written as RFC 3339 (`2024-05-01T10:30:00Z`) or as values of HTML date inputs (`2024-05-01`, `2024-05-01T10:30`), see [Times](../struct.md#3-times). For example, listing the meetings from a date picked by the user, for a request like `/meetings?from=2024-05-01`:

```go
objs, _, err := svc.List(ctx, "meetings", 50, 0, "StartsAt", "asc",
    map[string]string{"StartsAt": r.URL.Query().Get("from")},
    map[string]string{"StartsAt": "ge"},
    nil, nil, nil)
```

For a pointer field, an empty value is `NULL`: `eq` with an empty value lists the rows where the field is `NULL`, and `ne` the rows where it is not. For example, the open tasks for `/tasks?completed_at=`:

```go
objs, _, err := svc.List(ctx, "tasks", 50, 0, "DueAt", "asc",
    map[string]string{"CompletedAt": r.URL.Query().Get("completed_at")},
    map[string]string{"CompletedAt": "eq"},
    nil, nil, nil)
```

**Returns** `*FilterValidationError` on an unknown operator or a filter field that fails struct validation.

---

### Num

```go
count, err := svc.Num(ctx, "users",
    map[string]string{"Status": "1"},
    map[string]string{"Status": "eq"},
)
```

Returns the count of records matching the given filters. Accepts the same `filterVals`/`filterOps` maps as `List`. Useful for pagination — combine with `List`'s `limit`/`offset` to get a total alongside a page of results.

---

### Helper methods

```go
obj  := svc.New("users")   // returns a new zero-value *User, or nil if key unknown
id   := svc.ID(obj)        // returns obj.ID as uint64
```

## Error types

| Type | Sentinel / constructor | When returned |
|---|---|---|
| `*ModelValidationError` | — | `Save`: struct field validation failed |
| `*FilterValidationError` | — | `List` / `Num`: unknown filter operator or filter validation failed |
| `NotFoundError` | `errors.Is(err, NotFoundError)` | `Read` / `Delete`: no record with the given ID |
| `InvalidKeyError` | `errors.Is(err, InvalidKeyError)` | Any method: key not found in the registry |

Both `ModelValidationError` and `FilterValidationError` carry a `Violations map[string]string` field mapping each failed field to the name of the failed validation rule, e.g. `"required"` or `"min"` from the [go-playground/validator](https://github.com/go-playground/validator) library, `"regexp"` (`gocrud.ViolationRegExp`) for a failed `crud_regexp` pattern, or `"type"` (`gocrud.ViolationType`) for a value of a wrong type or an invalid filter operator.

## Example

```go
svc := svccrud.New(map[string]func() interface{}{
    "users": func() interface{} { return &User{} },
    "notes": func() interface{} { return &Note{} },
}, dbConn, gocrud.DialectPostgres)

// In an HTTP handler:
objs, err := svc.List(ctx, "notes", 20, 0, "CreatedAt", "desc",
    map[string]string{"UserID": r.Header.Get("X-User-ID")},
    map[string]string{"UserID": "eq"},
    nil, // rowFunc
    nil, // constructor (use registered default)
    nil, // passFieldsToVerify
)
```

## Transactions

`RunInTx`, `RunInTxWithOptions`, `WithTx` and `Tx` run several service calls in one database transaction:

```go
err := svc.RunInTx(ctx, func(tx *svccrud.CRUD) error {
    if err := tx.Save(ctx, task, userID); err != nil {
        return err
    }
    return tx.Save(ctx, activity, userID)
})
```

See [Transactions](../crud/transactions.md#service-layer) for the details and a complete HTTP handler example.
