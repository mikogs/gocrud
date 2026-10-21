# Operations

This page describes the methods of `CRUD`. Most of them also have a typed version, e.g. `gocrud.Get[User]` returns `[]*User` instead of `[]interface{}`, see [Typed functions](generics.md).

## Save

The `Save` method handles both creating new records and updating existing ones based on the presence of an identifier.

**Signature**

```go
func (c *CRUD) Save(ctx context.Context, obj interface{}, options SaveOptions) error
```

**Behavior**

* **Default Behavior (Upsert)**: By default, `Save` attempts to insert a new record. If a record with the same `ID` already exists in the database, it automatically falls back to performing an update.
* **Update Only**: To force an update operation without attempting an insertion (if the ID is known to exist), set the `NoInsert` flag to `true` in the options.

**Parameters**

* `ctx context.Context`: The execution context for the operation.
* `obj interface{}`: The object instance containing the data to be saved.
* `options SaveOptions`: Configuration flags for the save operation.

**SaveOptions**

```go
type SaveOptions struct {
    NoInsert   bool      // If true, skips the INSERT attempt and performs only UPDATE.
    ModifiedBy uint64    // Optional ID of the user performing the modification.
    ModifiedAt time.Time // Optional time of the modification, the current time when zero.
}
```

When the struct has the `CreatedAt`, `CreatedBy`, `ModifiedAt` and `ModifiedBy` fields, `Save` fills them in. The creation fields are set only when the object is inserted, and are never changed by an update. See [Creation and modification fields](../struct.md#4-creation-and-modification-fields).

## Delete

The `Delete` method removes a record from the database based on the identifier within the provided object.

**Signature**

```go
func (c *CRUD) Delete(ctx context.Context, obj interface{}, options DeleteOptions) error
```

**Behavior**

* **Removal**: Executes a database deletion for the record matching the `ID` in obj.
* **State Reset**: Upon successful removal, the obj instance passed to the function is modified in-place. All its fields are reset to their zero values (e.g., 0, "", nil, false), effectively clearing the object's state in memory.

**Parameters**

* `ctx context.Context`: The execution context for the operation.
* `obj interface{}`: The object instance representing the record to be deleted.
* `options DeleteOptions`: Currently an empty struct reserved for future configuration.

**DeleteOptions**

```go
type DeleteOptions struct {
    // Reserved for future options (e.g., soft delete flags, cascade settings)
}
```

## Load

The `Load` method retrieves a record from the database based on a specific ID and populates the provided struct instance with the data.

**Signature**

```go
func (c *CRUD) Load(ctx context.Context, obj interface{}, id string, options LoadOptions) *LoadOutput
```

**Behavior**

* **Data Retrieval**: Fetches the row corresponding to the provided id and maps the database columns to the fields of the obj struct.
* **Password Field Zeroing**: Any field tagged `crud:"pass"` is reset to `""` after the row is scanned. The bcrypt hash is never left in memory after a successful load.
* **Password Verification**: For every field tagged `crud:"pass"` that also appears as a key in `options.VerifyPasswordFields`, the corresponding map value is checked as a plaintext password against the bcrypt hash stored in that field, and the result is recorded in the returned `LoadOutput.PasswordFields` map. Keys in `options.VerifyPasswordFields` that do not name an actual password field are ignored and do not appear in the result.
* **Not Found Handling**: If no record exists with the given id, the method returns no error (typically), but the obj instance is zeroed out. This means all fields in the struct are reset to their default zero values (e.g., 0, "", nil, false), ensuring the caller receives a clean, empty object rather than partial or stale data.

**Parameters**

* `ctx context.Context`: The execution context for the operation.
* `obj interface{}`: The target struct instance to be populated with data. It must be a pointer to a struct.
* `id string`: The unique identifier of the record to load.
* `options LoadOptions`: Configuration options for the load operation.

**LoadOptions**

```go
type LoadOptions struct {
    // VerifyPasswordFields maps a struct field name to a plaintext password
    // to verify against the bcrypt hash stored in that field.
    VerifyPasswordFields map[string]string

    // Preload contains the relation fields whose related objects are loaded,
    // e.g. []string{"Customer", "Items.Product"}, see Relations.
    Preload []string
}
```

**LoadOutput**

```go
type LoadOutput struct {
    Error          error
    PasswordFields map[string]int // PassOK or PassInvalid, keyed by password field name
}
```

* `PassOK`: the supplied plaintext matches the stored bcrypt hash.
* `PassInvalid`: the supplied plaintext does not match the stored bcrypt hash.

## Get

The `Get` method retrieves a collection of objects from the database. Unlike `Load`, which fetches a single record by `ID`, `Get` supports complex filtering, sorting, pagination, and result transformation.

**Signature**

```go
func (c *CRUD) Get(
    ctx context.Context, 
    newObjFunc func() interface{}, 
    options GetOptions
) ([]interface{}, map[string]int, error)
```

**Behavior**

* **Dynamic Instantiation**: Uses `newObjFunc` to create a fresh instance of the target struct for every row fetched. This ensures type safety and avoids shared state issues.
* **Result Set**: Returns a slice of `interface{}` containing the populated objects (or transformed results).
* **Password verification**: The second result holds `PassOK` or `PassInvalid` for each field named in `VerifyPasswordFields`, when exactly one record is found, and is empty otherwise.
* **Filtering & Sorting**: Supports standard SQL-like filtering (`WHERE`), ordering (`ORDER BY`), and pagination (`LIMIT/OFFSET`).
* **Transformation**: Allows custom processing of each row before it is added to the result slice via `RowObjTransformFunc`.
* **Password Field Zeroing**: Any field tagged `crud:"pass"` is reset to `""` on every scanned row, before the transform function or the result slice is populated. Bcrypt hashes are never left in memory after a read.

**Parameters**

* `ctx context.Context`: Execution context.
* `newObjFunc func() interface{}`: A factory function that returns a pointer to a new struct instance (e.g., `func() interface{} { return &MyStruct{} }`).
* `options GetOptions`: Configuration for the query.

**GetOptions**

```go
type GetOptions struct {
    // Sorting: List of alternating field names and directions ("ASC" or "DESC")
    // Example: []string{"Age", "ASC", "Name", "DESC"}
    Order []string

    // Pagination
    Limit  int   // Maximum number of rows to return (SQL LIMIT)
    Offset int   // Number of rows to skip (SQL OFFSET)

    // Filtering
    Filters *sqlfilters.Filters // Structured WHERE clause conditions

    // Transformation
    RowObjTransformFunc func(interface{}) interface{} // Custom function applied to every row

    // Type Conversion
    ConvertFiltersFromString bool // Converts string filter values to target field types (useful for URL params)

    // Passwords to verify against the stored hashes, by field name, when exactly one record is found
    VerifyPasswordFields map[string]string

    // Relation fields whose related objects are loaded, e.g. []string{"Customer", "Items.Product"}
    Preload []string
}
```

* **Sorting (Order)**: Accepts a flat list of strings where pairs represent Field and Direction.
  * Example: `[]string{"Age", "ASC", "Price", "DESC"}` sorts by `Age` ascending, then `Price` descending.

* **Filtering (Filters)**: Uses `sqlfilters.Filters` to build `WHERE` clauses.
  * **Standard Filters**: Map field names to operators (`Op`) and values (`Val`). A `nil` value with `OpEqual` or `OpNotEqual` is an `IS NULL` or `IS NOT NULL` condition, see [Optional values](../struct.md#5-optional-values-null).
  * **Raw SQL (`sqlfilters.Raw`)**: A fragment of SQL for conditions that standard filters cannot express, e.g. `".Price > ? OR .Stock = ?"`. Its `Op` is `sqlfilters.OpAND` or `sqlfilters.OpOR`, which joins it with the standard filters, and its `Val` is an `[]interface{}` with the SQL followed by the values, e.g. `{Op: sqlfilters.OpOR, Val: []interface{}{".Price > ?", 100}}`. Fields are written as `.FieldName` and replaced with their columns, and the values for `?` are passed as query parameters, so they are safe. The SQL text itself is used as it is, so never build it from user input.

  The operators of standard filters:

  | Operator | SQL | Value |
  |---|---|---|
  | `OpEqual` | `=`, or `IS NULL` for `nil` | a value of the field type |
  | `OpNotEqual` | `!=`, or `IS NOT NULL` for `nil` | a value of the field type |
  | `OpGreater`, `OpGreaterOrEqual` | `>`, `>=` | a value of the field type |
  | `OpLower`, `OpLowerOrEqual` | `<`, `<=` | a value of the field type |
  | `OpLike` | `LIKE` | a pattern, e.g. `"%smith%"` |
  | `OpMatch` | `~` (PostgreSQL only) | a regular expression |
  | `OpBit` | `& value > 0` | a bit mask |
  | `OpIn` | `IN (...)` | a slice or an array of values of the field type |

  `OpIn` matches any of the values in the list. Each value is validated like the value of the field. An empty list matches nothing.

  ```go
  orders, err := gocrud.Get[Order](ctx, crud, gocrud.GetOptions{
      Filters: &sqlfilters.Filters{
          "Status":     {Op: sqlfilters.OpIn, Val: []string{"paid", "shipped"}},
          "CustomerID": {Op: sqlfilters.OpIn, Val: customerIDs}, // e.g. []uint64{3, 8, 21}
      },
  })
  ```

  With `ConvertFiltersFromString`, the values of `OpIn` are separated with commas, e.g. `"paid,shipped"`, and an empty string is an empty list. A value cannot contain a comma.

  Each value of `OpIn` is a parameter of the query, and databases limit their number: 65535 in PostgreSQL and 32766 in SQLite.

* **Type Conversion (ConvertFiltersFromString)**: Automatically casts string inputs (e.g., from HTTP query parameters) to the correct Go data types required by the database columns. Times can be written in several formats, e.g. `2024-05-01T10:30:00Z` or `2024-05-01`, see [Times](../struct.md#3-times).

* **Row Transformation (RowObjTransformFunc)**: Instead of returning raw structs, you can transform each row into a different format (e.g., a map for JSON, a string for CSV, or HTML snippets).

* **Related objects (Preload)**: Loads the related objects of the relation fields, with one query per relation, see [Relations](relations.md#loading-related-objects).



## GetCount

The `GetCount` method returns the total number of rows in a database table, optionally filtered by specific conditions.

**Signature**

```go
func (c *CRUD) GetCount(ctx context.Context, obj interface{}, options GetCountOptions) (uint64, error)
```

**Behavior**

* Executes a `SELECT COUNT(*)` query against the table associated with the provided struct type.
* Returns the number of matching rows as a uint64.
* Without filters, returns the total row count for the entire table.

**Parameters**

* `ctx context.Context`: Execution context.
* `obj interface{}`: A struct instance (or pointer) used to identify the target database table. The struct's field values are not used for filtering — only its type determines which table to query.
* `options GetCountOptions`: Configuration for filtering the count query.

**GetCountOptions**

```go
type GetCountOptions struct {
    Filters                  *sqlfilters.Filters // Structured WHERE clause conditions
    ConvertFiltersFromString bool                // Converts string filter values to target field types
}
```

* **Filtering (Filters)**: Uses the same `sqlfilters.Filters` mechanism as `Get`, allowing you to count only rows that match specific conditions. This is particularly useful for pagination — combining GetCount with `Get`'s `Limit`/`Offset` gives you total records alongside a paginated subset.

* **Type Conversion (`ConvertFiltersFromString`)**: Identical behavior to `Get`. When enabled, string-based filter values (e.g., from URL query parameters) are automatically converted to the correct Go data types before being applied to the query.

## DeleteMultiple

The `DeleteMultiple` method removes multiple records from the database table that match specific filter criteria, similar to how `Get` retrieves them.

**Signature**

```go
func (c *CRUD) DeleteMultiple(ctx context.Context, obj interface{}, options DeleteMultipleOptions) ([]uint64, error)
```

**Behavior**

* **Batch Deletion**: Executes a `DELETE` query based on the `Filters` provided, removing all rows that satisfy the conditions.
* **Deleted IDs**: Returns the IDs of the deleted rows, in the order the database returns them, or an empty slice when no rows matched. The rows of other tables that are deleted by foreign keys are not included.
* **Related rows**: Rows of other tables that reference the deleted rows are deleted, updated or protected by the database, according to their foreign keys, see [Foreign keys](relations.md#foreign-keys).

**Parameters**

* `ctx context.Context`: Execution context.
* `obj interface{}`: A struct instance used to identify the target table.
* `options DeleteMultipleOptions`: Configuration for the batch deletion.

**DeleteMultipleOptions**

```go
type DeleteMultipleOptions struct {
    Filters            *sqlfilters.Filters // Conditions to select rows for deletion
}
```

## UpdateMultiple

The `UpdateMultiple` method updates multiple records in the database table that match specific filter criteria. It allows selective field updates across all matching rows.

**Signature**

```go
func (c *CRUD) UpdateMultiple(ctx context.Context, obj interface{}, fieldsToUpdate map[string]interface{}, options UpdateMultipleOptions) error
```

**Behavior**

* **Batch Update**: Executes an `UPDATE` query that modifies only the specified fields for all rows matching the filter conditions.
* **Selective Updates**: Only the fields listed in fieldsToUpdate are modified; all other fields remain unchanged.
* **Filtering**: Uses `sqlfilters.Filters` to determine which rows should be updated, similar to Get and DeleteMultiple.

**Parameters**

* `ctx context.Context`: Execution context.
* `obj interface{}`: A struct instance used to identify the target table.
* `fieldsToUpdate map[string]interface{}`: A map where keys are field names and values are the new values to set.
* `options UpdateMultipleOptions`: Configuration for the batch update.

**UpdateMultipleOptions**

```go
type UpdateMultipleOptions struct {
    Filters                 *sqlfilters.Filters // Conditions to select rows for update
    ConvertValuesFromString bool                // Converts string values to target field types
}
```

* Field Mapping (`fieldsToUpdate`):
  * Keys are the field names as they appear in the struct (matching database column names).
  * Values are the new values to assign.
  * Only these fields are updated; others remain untouched.

* Type Conversion (`ConvertValuesFromString`):
  * When enabled, string values in fieldsToUpdate are automatically converted to the correct Go data types before being written to the database.
  * Useful when values come from external sources (e.g., HTTP form data, URL parameters).


## Transactions

All the operations above can run in a database transaction, so that several changes are saved together or not at all. See [Transactions](transactions.md).

## Validating without saving

The operations validate objects and filters themselves, but the validation can also be run on its own, e.g. to show the errors of a form before anything is saved:

```go
// all the fields of obj
ok, violations, err := gocrud.Validate(obj, nil, "crud")

// only the named fields
ok, violations, err = gocrud.Validate(obj, map[string]bool{"Email": true}, "crud")

// values as UpdateMultiple validates them, without changing obj
ok, violations, err = gocrud.ValidateValues(obj, map[string]interface{}{"Email": "x"}, "crud")

// filters as Get validates them, returns a *gocrud.CRUDError with a *gocrud.ValidationError
err = gocrud.ValidateFilters(obj, filters, "crud")
```

`violations` maps the names of the invalid fields to the names of the failed rules, see [Validation Errors](../struct.md#validation-errors). The last argument is the `TagName` of the CRUD, which is used for the `<tagName>_regexp` tags. `err` is set when `obj` cannot be validated, e.g. it is not a pointer to a struct.

## Errors

Most errors are a `*gocrud.CRUDError`, whose `Op` describes the step that failed, e.g. `"database exec"`, and whose `Err` is the cause. `UniqError`, and `NotFoundError` of `Load[T]`, are returned as they are. Use `errors.Is` and `errors.As` to check any of them:

| Error | When |
|---|---|
| `*gocrud.ValidationError` | the object or a filter is not valid, `Violations` has the failed rules |
| `*gocrud.UniqError` | a unique field already has the value, see below |
| `gocrud.ForeignKeyViolationError` | a foreign key is violated, see [Relations](relations.md#violations) |
| `gocrud.NotFoundError` | `Load[T]` did not find the object |
| `gocrud.PreloadError` | a name in `Preload` is not a relation field |
| `gocrud.UnsupportedOptionError` | an option is not supported by a [typed function](generics.md) |
| `gocrud.AliasedColumnsMigrateError` | `Migrate` of a struct with [column aliases](../struct.md#column-aliases) |
| `structinfo.NotStructPointerError`, `IDFieldError`, `NoColumnsError`, `PasswordFieldError`, `RelationError`, `ForeignKeyError` | the struct cannot be used, see [Struct](../struct.md) |

The `Field` of `UniqError` depends on what detected the duplicate:

* the name of the field, when it was found by the `GetCountOnUniq` check, e.g. `Email`,
* the name of the constraint in PostgreSQL, e.g. `user_email_key`,
* the table and the column in SQLite, e.g. `user.email`.
