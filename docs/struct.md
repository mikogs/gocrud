# Struct

To utilize the library's automatic CRUD capabilities, your Go structs must adhere to a specific naming convention and tagging strategy. These definitions allow the library to map your data structures to database records while enforcing validation rules automatically.

## Basic requirements

### 1. The ID Field

Every struct intended for CRUD operations must have an `ID` field of type `uint64` as its first field, followed by at least one other field. This serves as the primary key for database operations. Structs are always passed as pointers, e.g. `&User{}`.

If a struct does not meet these requirements, every operation returns an error that can be checked with `errors.Is`, e.g. `errors.Is(err, structinfo.IDFieldError)`.

### 2. Supported Data Types

At this stage, the library supports the following basic data types for struct fields:

* Integers: int, int8, int16, int32, int64
* Unsigned Integers: uint, uint8, uint16, uint32, uint64
* Floats: float32, float64
* Strings: string
* Booleans: bool
* Times: `time.Time`, see [Times](#3-times)
* Pointers to any of the above, e.g. `*string` or `*time.Time`, for values that can be `NULL`, see [Optional values](#5-optional-values-null)

Complex types (such as slices, maps, or nested structs) are not currently supported for direct mapping in this context. Fields of such types, as well as unexported fields, are not stored in the database.

### 3. Times

`time.Time` fields are stored in the following columns:

| Database | Column type | Precision |
|---|---|---|
| PostgreSQL | `TIMESTAMPTZ` | microseconds, nanoseconds are rounded |
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

When filter values are strings, e.g. with `ConvertFiltersFromString` or in the service layer, a time can be written as:

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

* `ModifiedAt` and `ModifiedBy` are set on every save, to the time of the save and to the user from `SaveOptions.ModifiedBy`. The time is the current time unless `SaveOptions.ModifiedAt` is set, e.g. when importing data. Without a user, e.g. in a background job, `ModifiedBy` is `0`.
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

**Values as strings.** An empty string is `NULL` for a pointer field, when values are converted from strings: with `ConvertFiltersFromString`, `UpdateMultiple` with `ConvertValuesFromString`, and in the [service](pkgs/service.md) layer. This matches an empty optional input in an HTML form. For a `*string` field, it means that an empty string cannot be set this way, only `NULL`.

**Unique fields.** A unique pointer field can be `NULL` in many rows, because `NULL` is not equal to another `NULL`. This is also how the `GetCountOnUniq` check treats it.

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

* `pass`: Marks a `string` field as a password. On **Save** the plain-text value is automatically bcrypt-hashed before it is written to the database. On **Load** and **Get** the field is zeroed after the row is scanned so that bcrypt hashes never appear in memory after a read. When using [gorestapi](https://github.com/mikogs/gorestapi), the field is omitted entirely from JSON responses.
* `uniq`: Adds a `UNIQUE` constraint to the field's column when the table is created.
* `idx`: Creates a database index on the field. Use `idx:<name>` to give the index an explicit name; fields sharing the same `<name>` are combined into a single composite index, in the order they're declared in the struct. A bare `idx` (no name) creates its own single-column index.
* `uidx`: Same as `idx`, but creates a `UNIQUE` index instead of a regular one. Use `uidx:<name>` for a composite unique index across multiple fields.
* `-`: The field is not stored in the database.
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
	"Phone":    "regexp", // gocrud.ViolationRegExp
	"Age":      "type",   // gocrud.ViolationType - value of a wrong type
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