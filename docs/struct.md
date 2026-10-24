# Struct

To utilize the library's automatic CRUD capabilities, your Go structs must adhere to a specific naming convention and tagging strategy. These definitions allow the library to map your data structures to database records while enforcing validation rules automatically.

## Basic requirements

### 1. The ID Field

Every struct intended for CRUD operations must have an `ID` field as its first field, followed by at least one other field. This serves as the primary key for database operations. Structs are always passed as pointers, e.g. `&User{}`.

The `ID` is one of:

| Type | Column | Assigned |
|---|---|---|
| `uint64` | `SERIAL` in PostgreSQL, `INTEGER` in SQLite | by the database, 1, 2, 3, … |
| `string` | `UUID` in PostgreSQL, `TEXT` in SQLite | by gocrud, a [UUIDv7](https://www.rfc-editor.org/rfc/rfc9562#name-uuid-version-7), e.g. `0190f5c3-6d1e-7b3a-9c4d-2e5f6a7b8c9d` |

```go
type Order struct {
    ID     string `json:"id"`
    Status string `json:"status"`
}
```

Use a UUID when IDs are visible to users, e.g. in URLs: numbers reveal how many records there are, and invite guessing the IDs of other records. A UUID can also be set before the object is saved, e.g. by a client that works offline. A UUIDv7 starts with the time, so new rows are added at the end of the index, as with numbers, and sorting by `ID` sorts by the time of creation.

`Save` generates the UUID of a new object, i.e. one with an empty `ID`. An `ID` that is set has to be a UUID, otherwise the validation fails with the `type` violation for `ID`. gocrud stores UUIDs in their standard lowercase form, and converts the UUIDs it gets, e.g. in `Load` or in filters, to it too.

If a struct does not meet these requirements, every operation returns an error that can be checked with `errors.Is`, e.g. `errors.Is(err, gocrud.ErrIDField)`.

### 2. Supported Data Types

The exported fields of the following types are stored in columns:

* Integers: int, int8, int16, int32, int64
* Unsigned Integers: uint, uint8, uint16, uint32, uint64
* Floats: float32, float64
* Strings: string
* Booleans: bool
* Times: `time.Time`, see [Times](#3-times)
* Named types of the types above, e.g. `type Status string`, see [Named types](#6-named-types)
* Types with `Scan` and `Value` methods, e.g. `sql.NullString` or a decimal type, see [Valuer types](#7-valuer-types)
* Bytes: `[]byte`, and named types of it, e.g. `json.RawMessage`, see [Bytes](#8-bytes)
* Pointers to any of the above, e.g. `*string` or `*time.Time`, for values that can be `NULL`, see [Optional values](#5-optional-values-null)

Fields of other types, e.g. slices, maps and structs, are stored only with the `json` option, as JSON, see [JSON](#9-json). Without it, they are not stored in the database, as unexported fields are not.

**Integer columns.** The integer columns of PostgreSQL are signed, so an unsigned field gets a column that holds all its values:

| Field | PostgreSQL | SQLite |
|---|---|---|
| `int8`, `int16`, `uint8` | `SMALLINT` | `INTEGER` |
| `int32`, `uint16` | `INTEGER` | `INTEGER` |
| `int`, `int64`, `uint32`, `uint`, `uint64` | `BIGINT` | `INTEGER` |

The largest integer of both databases is 2^63-1, so a `uint64` from 2^63, e.g. `math.MaxUint64`, cannot be saved, and `Save` returns an error.

**Values of filters** have to fit in the type of the field, otherwise they are the [violation](#validation-errors) `type`, e.g. `300` or `-1` for a `uint8`, or `1.5` for an `int64`. A number that would change in the conversion to the type of the field could pass the `validate` rules as another number.

### 3. Times

`time.Time` fields are stored in the following columns:

| Database | Column type | Precision |
|---|---|---|
| PostgreSQL | `TIMESTAMPTZ` | microseconds, nanoseconds are rounded by lib/pq and cut off by pgx |
| SQLite | `DATETIME` | nanoseconds |

SQLite has no date and time type, so times are stored as text in UTC, in a format with a fixed length: `2024-05-01 10:30:00.000000000+00:00`. This way, sorting and comparing the text gives the same result as sorting and comparing the times.

**Time zones.** A time is saved as a point in time, so `10:00` in Warsaw and `08:00` in UTC are the same value, and filters and sorting work across time zones. When loaded, times are always in UTC. Convert them to the user's time zone for display:

```go
local := meeting.StartsAt.In(userLocation)
```

**Times that are not set.** The column default is the zero value of `time.Time`, so a time that was never set is loaded as the zero value. Check it with `IsZero()`, and use the `required` validation rule when a time must be set.

```go
type Meeting struct {
    ID       uint64    `json:"id"`
    Title    string    `json:"title" validate:"required,max=200"`
    StartsAt time.Time `json:"starts_at" validate:"required" crud:"idx"`
    EndsAt   time.Time `json:"ends_at" validate:"omitempty,gtfield=StartsAt"` // optional, but after StartsAt
}
```

**Filters.** Use `time.Time` values with any operator. A filter map holds one condition per field, so a range of times is two maps joined with `And`, see [Filters](crud/ops.md#filters):

```go
// the meetings that start within the next 7 days, soonest first
now := time.Now()
meetings, err := gocrud.Get[Meeting](ctx, crud, gocrud.GetOptions{
    Order: []string{"StartsAt", "asc"},
    Filters: sqlfilters.And(
        sqlfilters.Filters{"StartsAt": {Op: sqlfilters.OpGreaterOrEqual, Val: now}},
        sqlfilters.Filters{"StartsAt": {Op: sqlfilters.OpLower, Val: now.AddDate(0, 0, 7)}},
    ),
})
```

When filter values are strings, e.g. with `ConvertFiltersFromString` or in the [registry](pkgs/registry.md), a time can be written as:

| Format | Example | Source |
|---|---|---|
| RFC 3339 | `2024-05-01T10:30:00+02:00`, `2024-05-01T08:30:00Z` | JSON, APIs |
| date and time | `2024-05-01T10:30:00`, `2024-05-01 10:30:00` | |
| date and time without seconds | `2024-05-01T10:30` | HTML `<input type="datetime-local">` |
| date | `2024-05-01` | HTML `<input type="date">` |

A time without a time zone is in UTC. Any other string is a validation error with the `type` violation.

**Your own SQL.** gocrud converts times for the database itself. When you pass a time to your own query, e.g. with `tx.Tx().ExecContext` in a [transaction](crud/transactions.md), use `TimeValue`, which returns the time in the form the database expects:

```go
_, err := tx.Tx().ExecContext(ctx,
    `UPDATE meeting SET ends_at = $1 WHERE id = $2`,
    tx.TimeValue(endsAt), meetingID)
```

### 4. Creation and modification fields

A struct that has all of the following fields gets them filled in by `Save`:

```go
type Note struct {
    ID         uint64    `json:"id"`
    Text       string    `json:"text" validate:"required"`
    CreatedAt  time.Time `json:"created_at"`
    CreatedBy  uint64    `json:"created_by"`
    ModifiedAt time.Time `json:"modified_at"`
    ModifiedBy uint64    `json:"modified_by"`
}
```

* `CreatedBy` and `ModifiedBy` hold the ID of a user: a `uint64`, or a `string`, e.g. the UUID of a user, or the ID from an external identity provider such as `auth0|42`. Both have to be of the same type, otherwise the struct cannot be used, see `gocrud.ErrModificationFields`.
* `ModifiedAt` and `ModifiedBy` are set on every save, to the time of the save and to the user from `SaveOptions.ModifiedBy`. The time is the current time unless `SaveOptions.ModifiedAt` is set, e.g. when importing data. Without a user, e.g. in a background job, `ModifiedBy` is `0` or `""`.
* `CreatedAt` and `CreatedBy` are set when the object is inserted, to the same values. Values set on the object are ignored, so a client cannot choose them, e.g. by sending `created_at` in JSON.
* `CreatedAt` and `CreatedBy` are **never changed by an update**. An object decoded from an HTTP request usually does not have them, and saving it does not erase them from the database.

After an update, the `CreatedAt` and `CreatedBy` fields of the object are not read from the database. When they were empty, `Save` sets them to the time and user of the update. Use `Load` to get the stored values.

If any of the four fields is missing or has a different type, all of them are ordinary fields.

### 5. Optional values (NULL)

A field of any of the types above can be `NULL` when it is a pointer, e.g. `*string`, `*int64` or `*time.Time`. Its column does not have `NOT NULL` and a default value, and a `nil` pointer is saved and loaded as `NULL`.

Use pointers when "no value" is different from the zero value: a score of `0` is not the same as no score yet, and `false` is not the same as not answered. Fields that are not pointers can never be `NULL`, and their zero values are stored instead.

```go
type Task struct {
    ID          uint64     `json:"id"`
    Title       string     `json:"title" validate:"required"`
    Estimate    *int64     `json:"estimate" validate:"omitempty,min=1"` // in hours, nil when not estimated
    DueAt       *time.Time `json:"due_at"`
    CompletedAt *time.Time `json:"completed_at"` // nil while the task is open
}
```

In JSON, `nil` is `null`, so a task that is not estimated is encoded as `"estimate": null`.

**Validation.** Rules are checked only when a pointer is not `nil` if they follow `omitempty`, like `Estimate` above. Use `required` when a pointer must not be `nil`. A regular expression in `crud_regexp` is checked only for a value that is not `NULL`.

**Filters.** A `nil` value with `OpEqual` is an `IS NULL` condition, and with `OpNotEqual` an `IS NOT NULL` condition:

```go
// open tasks that are due before the end of the week
openTasks, err := gocrud.Get[Task](ctx, crud, gocrud.GetOptions{
    Order: []string{"DueAt", "asc"},
    Filters: &sqlfilters.Filters{
        "CompletedAt": {Op: sqlfilters.OpEqual, Val: nil},
        "DueAt":       {Op: sqlfilters.OpLower, Val: endOfWeek},
    },
})
```

`nil` with any other operator, and on a field that is not a pointer, is a validation error with the `type` violation. As always in SQL, a comparison with `NULL` is never true, so `DueAt < endOfWeek` does not match the tasks without a due date. To include them, join the conditions with `Or`:

```go
sqlfilters.Or(
    sqlfilters.Filters{"DueAt": {Op: sqlfilters.OpLower, Val: endOfWeek}},
    sqlfilters.Filters{"DueAt": {Op: sqlfilters.OpEqual, Val: nil}},
)
```

**Values as strings.** An empty string is `NULL` for a pointer field, when values are converted from strings: with `ConvertFiltersFromString`, `UpdateMultiple` with `ConvertValuesFromString`, and in the [registry](pkgs/registry.md). This matches an empty optional input in an HTML form. For a `*string` field, it means that an empty string cannot be set this way, only `NULL`.

**Unique fields.** A unique pointer field can be `NULL` in many rows, because `NULL` is not equal to another `NULL`. This is also how the `GetCountOnUniq` check treats it.

### 6. Named types

A field of a named type of a basic type, e.g. `type Status string` or `type Level int`, gets the column of the basic type: `Level` is stored in an integer column. Filters and `UpdateMultiple` take values of the named type, or of its basic type, e.g. `"open"` for a `Status`, and the `validate` rules work as for the basic type:

```go
type Status string

type Level int

type Ticket struct {
    ID       uint64 `json:"id"`
    Status   Status `json:"status" validate:"oneof=open closed"`
    Priority *Level `json:"priority"` // NULL when not set
}

tickets, err := gocrud.Get[Ticket](ctx, crud, gocrud.GetOptions{
    Filters: sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "open"}},
})
```

### 7. Valuer types

A field of a type that `database/sql` reads with a `Scan` method and writes with a `Value` method is stored as the type writes itself: a type whose pointer is a `sql.Scanner`, and which is, or whose pointer is, a `driver.Valuer`. Such types are e.g. `sql.NullString`, `sql.NullInt64`, `sql.NullTime`, `sql.Null[T]`, and the types of UUIDs and decimals of other packages.

**The column** is the column of what the zero value of the type is written as, e.g. a text column for a decimal type that writes `"0.00"`, and its default is the zero value, e.g. `'0.00'`, so that `Migrate` can add the column to a table with rows. The type of the column can be changed with the [`type:` option](#the-crud-tag), and its default stays the zero value:

```go
// Decimal is a decimal type, e.g. of another package, which writes itself as text, e.g. "0.00"
type Payment struct {
    ID     uint64         `json:"id"`
    Amount Decimal        `json:"amount" crud:"type:numeric(12,2)"` // NUMERIC(12,2) NOT NULL DEFAULT '0.00'
    Refund *Decimal       `json:"refund" crud:"type:numeric(12,2)"` // NULL when there is no refund
    Note   sql.NullString `json:"note"`                             // VARCHAR(255) in PostgreSQL, nullable
    PaidAt sql.NullTime   `json:"paid_at"`                          // TIMESTAMPTZ in PostgreSQL, nullable
}
```

* **NULL.** A type whose zero value is written as `NULL`, e.g. `sql.NullString`, is stored in a nullable column, like a pointer. A struct of a value and `Valid`, such as `sql.NullInt64`, gets the column of its value, e.g. `BIGINT`. Another type whose zero value is `NULL` needs a `type:` option, otherwise the struct cannot be used, see `gocrud.ErrColumnType`.
* **Times** of Valuer types, e.g. of `sql.NullTime`, are stored in SQLite in the same format as `time.Time` fields, so that they are compared and sorted in the same way. They are not converted to UTC when they are loaded: a Valuer has the time that it scans.
* **Filters and `UpdateMultiple`** take values of the type, or values of other types that the type can scan, as if they were read from the database, e.g. `"12.50"` or `12.5` for a decimal, an `int` for a `sql.NullInt64`, a `time.Time` for a `sql.NullTime`, or a UUID in upper case for a type of UUIDs. They are converted to the type, so they are written as the values of the field are, e.g. a UUID in its standard form. A value that the type cannot scan is the [violation](#validation-errors) `type`. `nil` is `NULL` for a type whose zero value is `NULL`.
* **Values from strings**, e.g. with `ConvertFiltersFromString` or in the [registry](pkgs/registry.md), are converted with the `UnmarshalText` method of the type, when it has one, e.g. for UUIDs, and otherwise scanned as the value that the type is written as: `"3"` is the number 3 for a `sql.NullInt64`, and `"2024-05-01"` a time for a `sql.NullTime`. An empty string is `NULL` for a type whose zero value is `NULL`.
* **`LIKE`** needs a pattern that the type can scan, as for the other types, e.g. a `sql.NullString`.
* **Unique fields.** `GetCountOnUniq` checks the values of Valuer types too, except `NULL`.

### 8. Bytes

A `[]byte` field, or a field of a named type of it, e.g. `json.RawMessage`, is stored in a `BYTEA` column in PostgreSQL, and a `BLOB` column in SQLite. The column is `NOT NULL`, with an empty default, and a `nil` slice is saved as an empty one, so it is loaded with no bytes. Use `*[]byte` for bytes that can be `NULL`.

Filters take `[]byte` values, e.g. `{"Hash": {Op: sqlfilters.OpEqual, Val: hash}}`. Values from strings are not converted to bytes, so they are the violation `type`.

### 9. JSON

A field with the `json` option of the `crud` tag is stored as JSON, with `encoding/json`. It can be of any type that `encoding/json` writes, e.g. a struct, a map or a slice, also with its own `MarshalJSON` and `UnmarshalJSON` methods:

```go
type Address struct {
    Street string `json:"street"`
    City   string `json:"city"`
}

type Customer struct {
    ID       uint64            `json:"id"`
    Address  Address           `json:"address" crud:"json"`  // JSONB NOT NULL DEFAULT '{"street":"","city":""}'
    Tags     []string          `json:"tags" crud:"json"`     // JSONB NOT NULL DEFAULT 'null'
    Previous *Address          `json:"previous" crud:"json"` // JSONB, NULL for nil
    Settings map[string]string `json:"settings" crud:"json type:json"`
}
```

* **The column** is `JSONB` in PostgreSQL and `TEXT` in SQLite, or the type of a `type:` option, e.g. `type:json` for the `JSON` type of PostgreSQL. Its default is the JSON of the zero value, so that `Migrate` can add it to a table with rows.
* **NULL.** A pointer field is nullable, and `nil` is `NULL`. The column of a field that is not a pointer is `NOT NULL`, so a `nil` map or slice is the JSON `null`, and a `nil` `json.RawMessage` is loaded as `null`.
* **The JSON text** is not kept as it is written by a `JSONB` column, which removes the spaces and orders the keys of objects. Use `type:json` to keep it.
* **Loading** decodes the JSON into a new value, so that the values of a reused object, e.g. the keys of a map, are replaced and not merged.
* **Filters** on a JSON field can only check whether a pointer field is `NULL`, with `nil` and `OpEqual` or `OpNotEqual`. Other conditions, and values from strings, are the violation `type`, and a JSON field cannot be compared with a [subquery](crud/ops.md#subqueries). Use a [raw filter](crud/ops.md#raw-sql) for conditions on the content of the JSON, e.g. with the JSON operators of PostgreSQL.
* **`UpdateMultiple`** takes values of the type of the field, or a pointer to it. A value of another type is the violation `type`.
* **Not allowed.** The `ID`, password fields and fields of [Valuer types](#7-valuer-types), which are written with their `Value` methods, cannot be stored as JSON, and neither can types that `encoding/json` cannot write, e.g. a struct with a func field. Such a struct cannot be used, see `gocrud.ErrColumnType`.

## Tagging

To configure field behavior, you will use three struct tags: `json`, `validate` and `crud`.

### The `json` Tag

Use the `json` tag to define the field name as it appears in JSON serialisation. This ensures your output follows standard naming conventions (e.g., snake_case) while your Go code uses idiomatic naming (PascalCase).

* **Format**: `json:"field_name"`
* **Example**: `json:"first_name"` maps the Go field FirstName to first_name in JSON.

### The `validate` Tag

The `validate` tag defines validation rules. They are enforced in the following scenarios:

* **Saving Data**: Before a record is saved to the database via the `Save` function.
* **Updating Data**: Values passed to `UpdateMultiple` are validated against the rules of the fields they update.
* **Filtering Data**: When using filters in the `Get`, `GetCount`, `UpdateMultiple` and `DeleteMultiple` methods to ensure query parameters are valid.

The rules are handled by the [go-playground/validator](https://github.com/go-playground/validator) library, so any of its [baked-in validators](https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-Baked_In_Validators_and_Tags) can be used. Rules are separated with a comma. The most common ones are:

* `required`: The field must not be a zero value (an empty string, `0`, `false`).
* `omitempty`: Skips the remaining rules when the field is a zero value. Use it for optional fields.
* `min=n`, `max=n`: For strings, the minimum and maximum length. For numbers, the minimum and maximum value.
* `len=n`: For strings, the exact length. For numbers, the exact value.
* `email`: Validates that the string is a properly formatted email address.
* `oneof=a b c`: The value must be one of the listed values.
* `-`: Skips the validation of the field.

### The `crud_regexp` Tag

Validates a `string` field against a regular expression. Regular expressions are kept in a separate tag, because characters such as `,` and `|` have a special meaning in the `validate` tag. An invalid pattern is reported as a failed validation.

* **Format**: `crud_regexp:"pattern"`
* **Example**: `crud_regexp:"^[a-z0-9_-]+$"`

**Note on Special Characters:** A backslash inside a struct tag must be escaped. For example: `crud_regexp:"^[0-9]{2}\\-[0-9]{3}$"`.

### The `crud` Tag

The `crud` tag defines how the field is stored in the database. Multiple options are separated with a space.

* `pass`: Marks a `string` field as a password. On **Save** the plain-text value is automatically bcrypt-hashed before it is written to the database, and the field is empty after the object is saved. On **Load** and **Get** the field is zeroed after the row is scanned so that bcrypt hashes never appear in memory after a read. An **empty** password is never hashed: an update keeps the stored password, so that an object can be loaded, changed and saved, and a new object has no password that can be verified, so use `validate:"required"` when a password is needed. A value that is a bcrypt hash is hashed like any other password. bcrypt hashes at most 72 bytes, so a longer password is the [violation](#validation-errors) `max`. When using [gorestapi](https://github.com/mikogs/gorestapi), the field is omitted entirely from JSON responses.
* `uniq`: Adds a `UNIQUE` constraint to the field's column when the table is created.
* `idx`: Creates a database index on the field. Use `idx:<name>` to give the index an explicit name; fields sharing the same `<name>` are combined into a single composite index, in the order they're declared in the struct. A bare `idx` (no name) creates its own single-column index.
* `uidx`: Same as `idx`, but creates a `UNIQUE` index instead of a regular one. Use `uidx:<name>` for a composite unique index across multiple fields.
* `-`: The field is not stored in the database.
* `json`: Stores the field as JSON, see [JSON](#9-json).
* `type:<column type>`: Overrides the type of the column, e.g. `type:numeric(12,2)`, `type:varchar(20)` or `type:text`. The type is a name of letters, digits and `_`, with an optional length, or a precision and a scale, in parentheses. Another type, e.g. `varchar(20);`, is an error that matches `gocrud.ErrColumnType`, as the type is a part of the SQL of the queries. The options are separated with spaces, so a type cannot have one: use e.g. `float8` instead of `double precision`. The default of the column stays the zero value of the field, e.g. `'0.00'` for a decimal type.
* `name:<column>`: Overrides the column name generated from the field name. Only lowercase letters, digits and `_` are allowed.
* `fk:<table>` and `on_delete:<action>`: Make the field a foreign key that references the `id` of another table, see [Foreign keys](crud/relations.md#foreign-keys).
* `rel:<key field>`: On a field that holds related objects, e.g. `Customer *Customer` or `Items []OrderItem`, which is not stored in a column. See [Relations](crud/relations.md).

### Column Aliases

A struct can read rows from more than one table when it builds its own SELECT queries by implementing `BuildSelectPrefixQuery` and `BuildSelectCountPrefixQuery`. In such a case, the column name can be prefixed with a table alias defined in that query, using `name:<alias>.<column>`. The alias is then used in the `WHERE` and `ORDER BY` clauses of `Get` and `GetCount`, so that filters on columns that exist in more than one table are not ambiguous.

All other queries (`CREATE TABLE`, `INSERT`, `UPDATE`, `DELETE` and the default `SELECT` queries) use the column name without the alias.

```go
type Product_WithPrice struct {
	ID        uint64 `json:"id" crud:"name:t1.id"`
	Name      string `json:"name" crud:"name:t1.name"`
	PriceName string `json:"price_name" crud:"name:t2.name"`
	Price     int64  `json:"price" crud:"name:t2.price"`
}

func (p *Product_WithPrice) BuildSelectPrefixQuery(tableNamePrefix string) (string, error) {
	return `SELECT t1.id, t1.name, t2.name, t2.price FROM "` + tableNamePrefix + `product" t1 ` +
		`INNER JOIN "` + tableNamePrefix + `price" t2 ON t1.id=t2.product_id`, nil
}

func (p *Product_WithPrice) BuildSelectCountPrefixQuery(tableNamePrefix string) (string, error) {
	return `SELECT COUNT(*) AS cnt FROM "` + tableNamePrefix + `product" t1 ` +
		`INNER JOIN "` + tableNamePrefix + `price" t2 ON t1.id=t2.product_id`, nil
}
```

The columns in the custom SELECT query must be in the same order as the struct fields.

### Validation Errors

When validation fails, the returned `*gocrud.ValidationError` carries a `Violations map[string]string` field. It maps the names of the struct fields that failed validation to the name of the rule that failed, for example:

```go
map[string]string{
	"Username": "min",
	"Email":    "email",
	"Phone":    "regexp",   // gocrud.ViolationRegExp
	"Age":      "type",     // gocrud.ViolationType - value of a wrong type
	"Password": "password", // gocrud.ViolationPassword - a filter on a password field
}
```

## Complete Example

Below is a comprehensive example of a `User` struct that demonstrates all the requirements and tagging strategies discussed.

```go
type User struct {
	// Required ID field (uint64)
	ID         uint64 `json:"id"`

	// Username: Required, length 3-30, must match regex
	Username   string `json:"username" validate:"required,min=3,max=30" crud_regexp:"^[a-z0-9_-]+$"`

	// Email: Required, must be valid email format, indexed for fast lookups
	Email      string `json:"email" validate:"required,email" crud:"idx"`

	// First Name: Required, length 2-50, part of a composite index with LastName
	FirstName  string `json:"first_name" validate:"required,min=2,max=50" crud:"idx:full_name"`

	// Last Name: Required, length 2-50, part of a composite index with FirstName
	LastName   string `json:"last_name" validate:"required,min=2,max=50" crud:"idx:full_name"`

	// Phone: Optional, length 7-20
	Phone      string `json:"phone" validate:"omitempty,min=7,max=20"`

	// Department: Optional, max length 100 (empty string allowed)
	Department string `json:"department" validate:"max=100"`

	// Role: Required, length 3-30
	Role       string `json:"role" validate:"required,min=3,max=30"`

	// IsActive: Boolean flag (no specific validation rules applied here)
	IsActive   bool   `json:"is_active"`

	// LastLogin: Must be 0 or a positive timestamp
	LastLogin  int64  `json:"last_login" validate:"min=0"`

	// Timestamps and Audit Fields
	CreatedAt  time.Time `json:"created_at"`
	CreatedBy  uint64    `json:"created_by"`
	ModifiedAt time.Time `json:"modified_at"`
	ModifiedBy uint64    `json:"modified_by"`
}
```