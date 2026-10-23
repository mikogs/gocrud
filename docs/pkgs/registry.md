# Registry

The `pkg/registry` package provides a higher-level wrapper around the core CRUD object. Where the core layer works directly with struct instances, the registry works with structs registered under keys — a map of string keys to constructor functions — and accepts filter values as plain strings, making it a natural fit for wiring up to HTTP handlers.

## Initialization

```go
import (
    "github.com/mikogs/gocrud"
    "github.com/mikogs/gocrud/pkg/registry"
)

reg := registry.New(
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
err := reg.CreateTables(ctx)
```

Iterates over all registered keys and calls `CREATE TABLE IF NOT EXISTS` for each struct, skipping projections, with the tables referenced by foreign keys first. To also update existing tables, use `Migrate` below.

### Migrating tables

```go
migrations, err := reg.Migrate(ctx)
```

Updates the tables of all the registered structs to match them, in a single transaction: creates the missing tables, adds the columns of new fields, and creates new indexes. Differences that could lose data, such as columns that are not fields of a struct, are not changed but logged as warnings and returned. See [Migrations](../crud/migrations.md).

```go
plans, err := reg.PlanMigrations(ctx)
```

Returns the same migrations, with the SQL statements in `Statements`, without changing anything. See [Planning a migration](../crud/migrations.md#planning-a-migration).

```go
err := reg.CheckSchema(ctx)
```

Returns a `*gocrud.SchemaError`, which matches `gocrud.ErrSchemaDiffers`, when the tables of the registered structs do not match them. See [Checking the schema](../crud/migrations.md#checking-the-schema).

## Methods

### Save

```go
err := reg.Save(ctx, obj, userID)
```

Saves `obj` to the database. If the `ID` is empty, `0` or `""`, an `INSERT` is performed, and a UUID is generated for a `string` ID; otherwise an upsert. `userID` is written to the `CreatedBy` and `ModifiedBy` fields, and the current time to `CreatedAt` and `ModifiedAt`, when the struct has them. It is a non-negative integer for fields of type `uint64`, and a string for fields of type `string`, or `nil` for no user. The creation fields are never changed by an update, see [Creation and modification fields](../struct.md#4-creation-and-modification-fields).

**Returns** `*ModelValidationError` when struct validation fails, otherwise a generic error.

---

### Read

```go
obj, passwordFields, err := reg.Read(ctx, "users", id, nil, nil)
```

Loads a single record by `id`. Returns `ErrNotFound` if no record with that ID exists.

The `id` is a number, or a decimal string, for an `ID` of type `uint64`, and a UUID for a `string` ID, so that the ID from a URL can be passed as it is. An ID of the wrong type, e.g. `abc` for a numeric ID, or a number for a UUID, returns `ErrInvalidID`.

The fourth argument is an optional constructor (`func() interface{}`). When `nil` the key's registered constructor is used. Pass a non-nil constructor to load the record into a different struct type — for example a read-specific projection with fewer fields, which will generate a narrower `SELECT`.

The fifth argument, `passFieldsToVerify`, maps a struct field name to a plaintext password to check against the stored hash. The returned `passwordFields` (`map[string]int`) holds `gocrud.PassOK` or `gocrud.PassInvalid` for each password field named in `passFieldsToVerify`; keys that do not name an actual password field are ignored.

```go
// The "_" suffix is stripped when deriving the table name:
//   strings.Split("User_Summary", "_")[0] = "User" → table "user"
type User_Summary struct {
    ID    uint64
    Email string
}

obj, _, err := reg.Read(ctx, "users", id, func() interface{} { return &User_Summary{} }, nil)
```

```go
// Verify a login password while loading the user record:
obj, passwordFields, err := reg.Read(ctx, "users", id, nil, map[string]string{
    "Password": submittedPassword,
})
if err == nil && passwordFields["Password"] == gocrud.PassOK {
    // password matches
}
```

---

### Delete

```go
err := reg.Delete(ctx, "users", id)
```

Reads the record by `id`, of the same types as in `Read`, first (returning `ErrNotFound` if absent), then deletes it. Rows of other tables that reference it are handled by the database, according to their [foreign keys](../crud/relations.md#foreign-keys).

---

### List

```go
objs, passwordFields, err := reg.List(
    ctx,
    "users",
    limit, offset,
    "Age", "asc",
    registry.Filters{
        Vals: map[string]string{"Status": "1"},
        Ops:  map[string]string{"Status": "eq"},
    },
    nil, // optional row transform func
    nil, // optional constructor override
    nil, // optional passwords to verify
)
```

Returns a paginated, filtered, ordered list of objects for the given key. All filter values are passed as strings and converted to the correct field types automatically, see [Filters](#filters).

**Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `key` | `string` | Registry key |
| `limit` | `int` | SQL `LIMIT` |
| `offset` | `int` | SQL `OFFSET` |
| `order` | `string` | Field name to sort by (empty = no ordering) |
| `orderDirection` | `string` | `"asc"` or `"desc"` |
| `filters` | `Filters` | The values and operators of the filters, see [Filters](#filters) |
| `rowFunc` | `func(interface{}) interface{}` | Optional per-row transform; `nil` returns raw structs |
| `constructor` | `func() interface{}` | Optional struct constructor; `nil` uses the key's registered constructor. Pass a non-nil value to scan rows into a different struct type (e.g. a list projection with fewer fields). |
| `passFieldsToVerify` | `map[string]string` | Optional passwords to verify, as in `Read`, when exactly one record is found |

**Returns** `*FilterValidationError` on an unknown operator, an invalid filter name, a filter value that fails struct validation, or an `order` that is not a field, with the `type` violation for it.

---

### Num

```go
count, err := reg.Num(ctx, "users", registry.Filters{
    Vals: map[string]string{"Status": "1"},
    Ops:  map[string]string{"Status": "eq"},
})
```

Returns the count of records matching the given filters. Accepts the same `Filters` as `List`. Useful for pagination — combine with `List`'s `limit`/`offset` to get a total alongside a page of results.

---

### Filters

`List` and `Num` take the filters as strings, e.g. from the query of a request:

```go
type Filters struct {
    Vals map[string]string // the values, by field name, e.g. "Status", path, e.g. "Customer.Country", or with a group, e.g. "Name~q"
    Ops  map[string]string // the operators, by the same keys, "eq" when not set

    ScopeVals map[string]string // conditions that every record has to match, by field name
    ScopeOps  map[string]string
}
```

An empty `Filters{}` matches all the records.

**Operators:**

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
| `in` | `IN (...)`, the values are separated with commas, e.g. `paid,shipped` |

Values of `time.Time` fields can be written as RFC 3339 (`2024-05-01T10:30:00Z`) or as values of HTML date inputs (`2024-05-01`, `2024-05-01T10:30`), see [Times](../struct.md#3-times). For example, listing the meetings from a date picked by the user, for a request like `/meetings?from=2024-05-01`:

```go
objs, _, err := reg.List(ctx, "meetings", 50, 0, "StartsAt", "asc", registry.Filters{
    Vals: map[string]string{"StartsAt": r.URL.Query().Get("from")},
    Ops:  map[string]string{"StartsAt": "ge"},
}, nil, nil, nil)
```

For a pointer field, an empty value is `NULL`: `eq` with an empty value lists the rows where the field is `NULL`, and `ne` the rows where it is not. For example, the open tasks for `/tasks?completed_at=`:

```go
objs, _, err := reg.List(ctx, "tasks", 50, 0, "DueAt", "asc", registry.Filters{
    Vals: map[string]string{"CompletedAt": r.URL.Query().Get("completed_at")},
    Ops:  map[string]string{"CompletedAt": "eq"},
}, nil, nil, nil)
```

**Paths.** A key can be a path through relation fields, e.g. `Customer.Country` for the customer of an order, or `Orders.Status` for the orders of a customer, see [Conditions on related records](../crud/ops.md#conditions-on-related-records). The `order` of `List` can be a path through belongs-to relations too, e.g. `Customer.Name`.

**Groups.** The conditions are joined with AND. A key can also name a group after a tilde, e.g. `Name~q` or `Customer.Name~q`. The conditions of a group are joined with OR, and the group is joined with AND with the other conditions and groups. The name of a group can be any text without a tilde, and only has to be the same for the conditions of the group. A key with an empty field, path segment or group name, or with a second tilde, is a `type` violation.

A search box that looks for the text in the name or the email, of the customers in a city:

```go
text := "%" + r.URL.Query().Get("q") + "%"
objs, _, err := reg.List(ctx, "customers", 50, 0, "Name", "asc", registry.Filters{
    Vals: map[string]string{"City": "Warsaw", "Name~q": text, "Email~q": text},
    Ops:  map[string]string{"Name~q": "like", "Email~q": "like"},
}, nil, nil, nil)
// WHERE "city"=$1 AND ("email" LIKE $2 OR "name" LIKE $3)
```

A group with one condition is joined with AND, so groups also give several conditions on one field, e.g. a range:

```go
Vals: map[string]string{"Total~min": "100", "Total~max": "500"},
Ops:  map[string]string{"Total~min": "ge", "Total~max": "lt"},
```

**Scope.** `ScopeVals` and `ScopeOps` are conditions that every record has to match, e.g. that it belongs to the signed-in user. They are keyed by field name or path, without groups. They are joined with AND around all the other conditions, so `Vals` cannot match records outside the scope, even with a group, and a condition in `Vals` on the same field only narrows the result:

```go
objs, _, err := reg.List(ctx, "notes", 20, 0, "CreatedAt", "desc", registry.Filters{
    Vals:      filterValsFromRequest, // anything the client sent
    Ops:       filterOpsFromRequest,
    ScopeVals: map[string]string{"UserID": userID},
}, nil, nil, nil)
```

---

### Helper methods

```go
obj   := reg.New("users")                // returns a new zero-value *User, or nil if key unknown
id    := reg.ID(obj)                     // returns obj.ID, a uint64 or a string with a UUID
keys  := reg.Registry()                  // returns the registry keys, in no particular order
names := reg.PasswordFieldNames("users") // returns the fields tagged with pass, or nil if key unknown
op, err := registry.Op("ge")              // returns sqlfilters.OpGreaterOrEqual, or ErrInvalidFilterOp
```

## Error types

| Type | Sentinel / constructor | When returned |
|---|---|---|
| `*ModelValidationError` | — | `Save`: struct field validation failed |
| `*FilterValidationError` | — | `List` / `Num`: unknown filter operator, invalid filter name, or filter validation failed |
| `ErrNotFound` | `errors.Is(err, ErrNotFound)` | `Read` / `Delete`: no record with the given ID |
| `ErrInvalidID` | `errors.Is(err, ErrInvalidID)` | `Read` / `Delete`: an ID of the wrong type, `Save`: a user ID of the wrong type |
| `ErrInvalidKey` | `errors.Is(err, ErrInvalidKey)` | Any method: key not found in the registry |
| `ErrInvalidFilterOp` | `errors.Is(err, ErrInvalidFilterOp)` | `Op`: unknown operator string. `List` and `Num` return a `*FilterValidationError` for it instead |
| `ErrForeignKeyCycle` | `errors.Is(err, ErrForeignKeyCycle)` | `CreateTables` / `Migrate`: foreign keys reference each other in a cycle |

Both `ModelValidationError` and `FilterValidationError` carry a `Violations map[string]string` field mapping each failed field to the name of the failed validation rule, e.g. `"required"` or `"min"` from the [go-playground/validator](https://github.com/go-playground/validator) library, `"regexp"` (`gocrud.ViolationRegExp`) for a failed `crud_regexp` pattern, `"type"` (`gocrud.ViolationType`) for a value of a wrong type, an invalid filter operator or an invalid order, or `"password"` (`gocrud.ViolationPassword`) for a filter on a password field, which cannot be used in filters.

## Example

```go
reg := registry.New(map[string]func() interface{}{
    "users": func() interface{} { return &User{} },
    "notes": func() interface{} { return &Note{} },
}, dbConn, gocrud.DialectPostgres)

// In an HTTP handler, the notes of the user:
objs, _, err := reg.List(ctx, "notes", 20, 0, "CreatedAt", "desc",
    registry.Filters{ScopeVals: map[string]string{"UserID": userID}},
    nil, // rowFunc
    nil, // constructor (use registered default)
    nil, // passFieldsToVerify
)
```

## Transactions

`RunInTx`, `RunInTxWithOptions`, `WithTx` and `Tx` run several registry calls in one database transaction:

```go
err := reg.RunInTx(ctx, func(tx *registry.CRUD) error {
    if err := tx.Save(ctx, task, userID); err != nil {
        return err
    }
    return tx.Save(ctx, activity, userID)
})
```

See [Transactions](../crud/transactions.md#registry) for the details and a complete HTTP handler example.
