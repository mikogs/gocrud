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

Complex types (such as slices, maps, or nested structs) are not currently supported for direct mapping in this context. Fields of such types, as well as unexported fields, are not stored in the database.

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
	CreatedAt  int64  `json:"created_at"`
	CreatedBy  uint64 `json:"created_by"`
	ModifiedAt int64  `json:"modified_at"`
	ModifiedBy uint64 `json:"modified_by"`
}
```