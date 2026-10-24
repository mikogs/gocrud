# Operations

This page describes the methods of `CRUD`. Most of them also have a typed version, e.g. `gocrud.Get[User]` returns `[]*User` instead of `[]interface{}`, see [Typed functions](generics.md).

## Save

The `Save` method handles both creating new records and updating existing ones based on the presence of an identifier.

**Signature**

```go
func (c *CRUD) Save(ctx context.Context, obj interface{}, options SaveOptions) error
```

**Behavior**

* **New objects**: An object with an empty `ID`, `0` or `""`, is inserted. The database assigns a numeric `ID`, and `Save` generates a UUID for a `string` ID, see [The ID Field](../struct.md#1-the-id-field). The `ID` is set on the object.
* **Default Behavior (Upsert)**: An object with an `ID` is inserted with that `ID`. If a record with the same `ID` already exists in the database, it automatically falls back to performing an update.
* **Update Only**: To force an update operation without attempting an insertion, set the `NoInsert` flag to `true` in the options. When there is no object with the `ID`, nothing is saved, and `Save` returns an error that matches `gocrud.ErrNotFound`.
* **Versions**: An object with a [version field](../struct.md#10-version) is updated only when it has the stored version, and otherwise `Save` returns an error that matches `gocrud.ErrVersionConflict`.
* **Password fields**: A field tagged `crud:"pass"` is hashed with bcrypt, and is empty after the object is saved, as after `Load`. An empty password keeps the stored password of an object that is updated, so an object can be loaded, changed and saved without changing its password. Set the field to a new password to change it. When `Save` fails, the field has the password again.

**Parameters**

* `ctx context.Context`: The execution context for the operation.
* `obj interface{}`: The object instance containing the data to be saved.
* `options SaveOptions`: Configuration flags for the save operation.

**SaveOptions**

```go
type SaveOptions struct {
    NoInsert   bool        // If true, skips the INSERT attempt and performs only UPDATE.
    ModifiedBy interface{} // Optional ID of the user performing the modification.
    ModifiedAt time.Time   // Optional time of the modification, the current time when zero.
}
```

`ModifiedBy` is a non-negative integer, or a decimal string, for `CreatedBy` and `ModifiedBy` fields of type `uint64`, and a string for fields of type `string`. Another value fails with an error that matches `gocrud.ErrInvalidID`.

When the struct has the `CreatedAt`, `CreatedBy`, `ModifiedAt` and `ModifiedBy` fields, `Save` fills them in. The creation fields are set only when the object is inserted, and are never changed by an update. See [Creation and modification fields](../struct.md#4-creation-and-modification-fields).

## InsertMultiple

`InsertMultiple` inserts many new objects of the same type with a few queries, e.g. when importing data, instead of a query per object:

```go
func (c *CRUD) InsertMultiple(ctx context.Context, objs []interface{}, options InsertMultipleOptions) error
func InsertMultiple[T any](ctx context.Context, c *CRUD, objs []*T, options InsertMultipleOptions) error

type InsertMultipleOptions struct {
    ModifiedBy interface{} // as in SaveOptions
    ModifiedAt time.Time   // as in SaveOptions, the current time when zero
}
```

```go
users := []*User{{Email: "a@example.com"}, {Email: "b@example.com"}}
err := gocrud.InsertMultiple(ctx, crud, users, gocrud.InsertMultipleOptions{})
// users[0].ID and users[1].ID are set
```

* **Each object** is prepared as a new object in `Save`: it is validated, gets a UUID for a `string` ID, its creation and modification fields, version `1` for a [version field](../struct.md#10-version), and its passwords are hashed. A `string` ID that is set is inserted, e.g. a UUID from another system. A `uint64` ID has to be `0`, as the database assigns it, otherwise the object fails with `gocrud.ErrInvalidID`.
* **The queries** insert up to 1000 rows each, fewer when the rows have more values than the database takes in a query: 65535 in PostgreSQL, and 32766 in SQLite. They run in a transaction, or in the transaction of a `CRUD` of `RunInTx` or `WithTx`, so that all the objects are inserted, or none.
* **The IDs** that the database assigns are set on the objects, in their order, and their password fields are empty, as after `Save`.
* **An error** leaves the objects as they were, e.g. without IDs and with their passwords, so that they can be fixed and inserted again. The error of an object, e.g. a [validation error](../struct.md#validation-errors), is a `*gocrud.ObjectError`, whose `Index` is the index of the object, and which matches the error of the object, e.g. with `errors.As(err, &validationErr)`. A duplicate value is a [`UniqError`](#errors), of no particular object.
* **Not done:** the `GetCountOnUniq` check of unique fields, as the `UNIQUE` constraints find the duplicates. A struct with its own [`InsertQuery`](custom-queries.md#query-methods) is inserted with it, one object at a time, in the transaction. Objects of different types are an error that matches `gocrud.ErrInsertMultiple`.

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
func (c *CRUD) Load(ctx context.Context, obj interface{}, id interface{}, options LoadOptions) (map[string]int, error)
```

**Behavior**

* **Data Retrieval**: Fetches the row corresponding to the provided id and maps the database columns to the fields of the obj struct.
* **Password Field Zeroing**: Any field tagged `crud:"pass"` is reset to `""` after the row is scanned. The bcrypt hash is never left in memory after a successful load.
* **Password Verification**: For every field tagged `crud:"pass"` that also appears as a key in `options.VerifyPasswordFields`, the corresponding map value is checked as a plaintext password against the bcrypt hash stored in that field, and the result is recorded in the returned map. Keys in `options.VerifyPasswordFields` that do not name an actual password field are ignored and do not appear in the result.
* **The time of a verification** is the same whether the object exists, has a password, or not: without a hash to check the password against, it is checked against a dummy hash, so that e.g. the time of a login does not show whether there is a user with its e-mail. The same applies to `Get` with `VerifyPasswordFields`, whatever the number of objects found.
* **Not Found Handling**: If no record exists with the given id, the method returns no error, but the obj instance is zeroed out. This means all fields in the struct are reset to their default zero values (e.g., 0, "", nil, false), ensuring the caller receives a clean, empty object rather than partial or stale data. Check it with `gocrud.ObjIDIsZero(obj)`, or use [`Load[T]`](generics.md#load), which returns `ErrNotFound`.

**Parameters**

* `ctx context.Context`: The execution context for the operation.
* `obj interface{}`: The target struct instance to be populated with data. It must be a pointer to a struct.
* `id interface{}`: The ID of the record: a non-negative integer, or a decimal string, for an `ID` of type `uint64`, and a UUID for a `string` ID, so that an ID from a URL can be passed as it is. Another value fails with an error that matches `gocrud.ErrInvalidID`.
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

**Returns** the results of `VerifyPasswordFields`, by the name of the password field:

* `PassOK`: the supplied plaintext matches the stored bcrypt hash.
* `PassInvalid`: the supplied plaintext does not match the stored bcrypt hash.

`PassInvalid` is `0`, the zero value of `int`, so a field that is not in the map, e.g. because no record was found, is never `PassOK`. Check the result with `== gocrud.PassOK`, e.g. `if passwords["Password"] == gocrud.PassOK`, and not with `!= gocrud.PassInvalid`.

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
* **Password verification**: The second result holds `PassOK` or `PassInvalid` for each field named in `VerifyPasswordFields`, when exactly one record is found, and is empty otherwise, so a field that is not in it is `PassInvalid`, e.g. for a login with an e-mail that no user has.
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
    Limit  int    // Maximum number of rows to return (SQL LIMIT)
    Offset int    // Number of rows to skip (SQL OFFSET)
    After  string // The cursor of NextCursor, see Pages with cursors below

    // Filtering, see Filters below
    Filters sqlfilters.Filter

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
  * **`ID` is the last field** of every order that does not have it, in the direction of the last field, e.g. `"Price", "DESC", "ID", "DESC"`, so that the objects with the same values of the other fields are always in the same order, and pages neither miss nor repeat any of them. An index of the fields and `ID`, e.g. `crud:"idx:price_id"` on both, serves such an order in both directions. A page, with a `Limit`, without an order is ordered by `ID`.
  * A field can be a path through belongs-to [relations](relations.md), e.g. `[]string{"Customer.Name", "asc", "ID", "asc"}` sorts orders by the names of their customers. Paths can be nested, e.g. `Order.Customer.Country` for the items of orders. Objects without the related object come last, in both directions and in both databases. A has-many relation, e.g. `Orders.Total` of a customer, has many values, and cannot be used to sort.
  * The related field is selected by a subquery, which finds the related object by its `ID`:

    ```sql
    ORDER BY (SELECT r1."name" FROM "customer" r1 WHERE r1."id" = "order"."customer_id") ASC NULLS LAST,"id" ASC
    ```

    Like [conditions on related records](#conditions-on-related-records), it cannot be used for a struct that builds its own `SELECT` with `BuildSelectPrefixQuery`.

* **Filtering (Filters)**: The conditions of the `WHERE` clause, e.g. `sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}}`, see [Filters](#filters).

* **Type Conversion (ConvertFiltersFromString)**: Converts the string values of the filters, e.g. from HTTP query parameters, to the types of the fields, see [Values from strings](#values-from-strings).

* **Row Transformation (RowObjTransformFunc)**: Instead of returning raw structs, you can transform each row into a different format (e.g., a map for JSON, a string for CSV, or HTML snippets).

* **Related objects (Preload)**: Loads the related objects of the relation fields, with one query per relation, see [Relations](relations.md#loading-related-objects).

### Pages with cursors

`Offset` makes the database read and skip all the rows before the page, so the pages of a large table get slower, and an object that is inserted or deleted while the pages are read moves the others to another page, so they are missed or repeated. A cursor instead marks the last object of a page, and the next page has the objects after it in the order:

```go
order := []string{"PublishedAt", "desc"}
page, err := gocrud.Get[Post](ctx, crud, gocrud.GetOptions{Order: order, Limit: 20})
// ...
if len(page) == 20 {
    after, err := crud.NextCursor(page[len(page)-1], order)
    // ...
    next, err := gocrud.Get[Post](ctx, crud, gocrud.GetOptions{Order: order, Limit: 20, After: after})
}
```

```go
func (c *CRUD) NextCursor(obj interface{}, order []string) (string, error)
```

* **The cursor** is a string for a URL, with the values of the order fields and the `ID` of the object, as they were loaded. It is encoded but not encrypted, so a client can read the values, also of fields that it is not shown otherwise.
* **The next page** has the objects after the cursor, with a condition on the order fields, together with the `Filters`: e.g. the `PublishedAt` is before the one of the cursor, or it is the same and the `ID` is lower. The condition also compares the first field on its own, e.g. `"published_at" <= $1`, so that the database can use its index. It reads only the rows of the page when there is an index of the order fields, see the order with `ID` above. The same `Order` and `Filters` are used for all the pages.
* **The last page** has fewer objects than `Limit`, or none. To know it without another query, get `Limit` plus one objects, and show `Limit` of them.
* **The order fields** of a cursor cannot be nullable, as `NULL` is neither greater nor lower than other values, JSON fields, or fields of related objects, otherwise `NextCursor` and `Get` return an error that matches `gocrud.ErrCursor`. So do a cursor of another order, a string that is not a cursor, and `After` with an `Offset`.
* **Only the next page.** There are no cursors of previous pages: keep the cursors of the pages that were read, e.g. in the URLs, to go back.



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
    Filters                  sqlfilters.Filter // Conditions, see Filters below
    ConvertFiltersFromString bool              // Converts string filter values to target field types
}
```

* **Filtering (Filters)**: The same [filters](#filters) as in `Get`, allowing you to count only rows that match specific conditions. This is particularly useful for pagination — combining GetCount with `Get`'s `Limit`/`Offset` gives you total records alongside a paginated subset.

* **Type Conversion (`ConvertFiltersFromString`)**: Identical behavior to `Get`. When enabled, string-based filter values (e.g., from URL query parameters) are automatically converted to the correct Go data types before being applied to the query.

## DeleteMultiple

The `DeleteMultiple` method removes multiple records from the database table that match specific filter criteria, similar to how `Get` retrieves them.

**Signature**

```go
func (c *CRUD) DeleteMultiple(ctx context.Context, obj interface{}, options DeleteMultipleOptions) ([]interface{}, error)
```

**Behavior**

* **Batch Deletion**: Executes a `DELETE` query based on the `Filters` provided, removing all rows that satisfy the conditions.
* **Deleted IDs**: Returns the IDs of the deleted rows, of the type of the `ID` field, `uint64` or `string`, in the order the database returns them, or an empty slice when no rows matched. The rows of other tables that are deleted by foreign keys are not included.
* **Related rows**: Rows of other tables that reference the deleted rows are deleted, updated or protected by the database, according to their foreign keys, see [Foreign keys](relations.md#foreign-keys).

**Parameters**

* `ctx context.Context`: Execution context.
* `obj interface{}`: A struct instance used to identify the target table.
* `options DeleteMultipleOptions`: Configuration for the batch deletion.

**DeleteMultipleOptions**

```go
type DeleteMultipleOptions struct {
    Filters sqlfilters.Filter // Conditions to select rows for deletion, see Filters below
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
* **Filtering**: The same [filters](#filters) as in `Get` and `DeleteMultiple` select the rows that are updated.
* **Password fields**: A password is hashed with bcrypt, once, so all the updated rows get the same hash. An empty password keeps the stored passwords, as in `Save`.
* **The map** `fieldsToUpdate` is not changed, also when the values are converted from strings or hashed.
* **Versions**: The [version field](../struct.md#10-version) of the updated rows is incremented, and a value of it in `fieldsToUpdate` is an error that matches `gocrud.ErrVersionField`.

**Parameters**

* `ctx context.Context`: Execution context.
* `obj interface{}`: A struct instance used to identify the target table.
* `fieldsToUpdate map[string]interface{}`: A map where keys are field names and values are the new values to set.
* `options UpdateMultipleOptions`: Configuration for the batch update.

**UpdateMultipleOptions**

```go
type UpdateMultipleOptions struct {
    Filters                 sqlfilters.Filter // Conditions to select rows for update, see Filters below
    ConvertValuesFromString bool              // Converts string values to target field types
}
```

* Field Mapping (`fieldsToUpdate`):
  * Keys are the field names as they appear in the struct (matching database column names).
  * Values are the new values to assign.
  * Only these fields are updated; others remain untouched.

* Type Conversion (`ConvertValuesFromString`):
  * When enabled, string values in fieldsToUpdate are automatically converted to the correct Go data types before being written to the database.
  * Useful when values come from external sources (e.g., HTTP form data, URL parameters).

## Filters

`Get`, `GetCount`, `UpdateMultiple` and `DeleteMultiple` select the rows with the `Filters` option, a `sqlfilters.Filter` from the package `github.com/mikogs/gocrud/pkg/sqlfilters`. A filter is one of:

| Filter | Matches the rows for which |
|---|---|
| `sqlfilters.Filters{…}`, or a pointer to it | all the conditions on fields in the map are true |
| `sqlfilters.And(f1, f2, …)` | all the filters are true |
| `sqlfilters.Or(f1, f2, …)` | at least one of the filters is true |
| `sqlfilters.Raw(sql, values…)` | a fragment of SQL is true |

A `nil` filter matches all the rows.

### Conditions on fields

`sqlfilters.Filters` maps the names of fields to an operator and a value. All the conditions in a map have to be true:

```go
orders, err := gocrud.Get[Order](ctx, crud, gocrud.GetOptions{
    Filters: sqlfilters.Filters{
        "Status":     {Op: sqlfilters.OpIn, Val: []string{"paid", "shipped"}},
        "CustomerID": {Op: sqlfilters.OpEqual, Val: customerID},
    },
})
```

| Operator | SQL | Value |
|---|---|---|
| `OpEqual` | `=`, or `IS NULL` for `nil` | a value of the field type |
| `OpNotEqual` | `!=`, or `IS NOT NULL` for `nil` | a value of the field type |
| `OpGreater`, `OpGreaterOrEqual` | `>`, `>=` | a value of the field type |
| `OpLower`, `OpLowerOrEqual` | `<`, `<=` | a value of the field type |
| `OpLike` | `LIKE` | a pattern, e.g. `"%smith%"` |
| `OpMatch` | `~` (PostgreSQL only) | a regular expression |
| `OpBit` | `& value > 0` | a bit mask |
| `OpIn` | `IN (...)` | a slice or an array of values of the field type, or a [subquery](#subqueries) |

* **`nil`** is valid only for a [nullable field](../struct.md#5-optional-values-null), with `OpEqual` or `OpNotEqual`.
* **A value of the field type** can also be a value of its basic type for a [named type](../struct.md#6-named-types), e.g. `"open"` for a `Status`, or a value that a [Valuer type](../struct.md#7-valuer-types) can scan, e.g. `"12.50"` for a decimal.
* **[JSON fields](../struct.md#9-json)** can only be compared with `nil`.
* **`OpIn`** matches any of the values in the list, and an empty list matches nothing. Each value is a parameter of the query, and databases limit their number: 65535 in PostgreSQL and 32766 in SQLite.

### Groups

A map has one condition for each field, joined with AND. `And` and `Or` combine filters in any other way, and can be nested:

```go
// paid orders, and shipped orders that are not delivered yet, of 100 to 500
orders, err := gocrud.Get[Order](ctx, crud, gocrud.GetOptions{
    Filters: sqlfilters.And(
        sqlfilters.Or(
            sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
            sqlfilters.Filters{
                "Status":      {Op: sqlfilters.OpEqual, Val: "shipped"},
                "DeliveredAt": {Op: sqlfilters.OpEqual, Val: nil},
            },
        ),
        sqlfilters.Filters{"Total": {Op: sqlfilters.OpGreaterOrEqual, Val: 100}},
        sqlfilters.Filters{"Total": {Op: sqlfilters.OpLower, Val: 500}},
    ),
})
```

```sql
WHERE ("status"=$1 OR ("delivered_at" IS NULL AND "status"=$2)) AND "total">=$3 AND "total"<$4
```

An empty `Or()` matches no rows, and an empty `And()` matches all the rows, so a group can be built in a loop:

```go
// customers with the text in any of the fields
search := sqlfilters.Or()
for _, field := range []string{"Name", "Email", "Phone"} {
    search.Filters = append(search.Filters, sqlfilters.Filters{field: {Op: sqlfilters.OpLike, Val: "%" + text + "%"}})
}

customers, err := gocrud.Get[Customer](ctx, crud, gocrud.GetOptions{Filters: search})
```

### Conditions on related records

A field can be a path through [relation fields](relations.md), the same as in `Preload`, e.g. `Customer.Country` for the customer of an order, or `Orders.Status` for the orders of a customer:

```go
// the paid orders of customers from Poland
orders, err := gocrud.Get[Order](ctx, crud, gocrud.GetOptions{
    Filters: sqlfilters.Filters{
        "Status":           {Op: sqlfilters.OpEqual, Val: "paid"},
        "Customer.Country": {Op: sqlfilters.OpEqual, Val: "PL"},
    },
})
```

```sql
WHERE EXISTS (SELECT 1 FROM "customer" r1 WHERE r1."id" = "order"."customer_id" AND r1."country"=$1) AND "status"=$2
```

* **Belongs to**, e.g. `Customer.Country`: the related object matches the condition. An object without it, e.g. with the key `0` or `NULL`, does not match.
* **Has many**, e.g. `Orders.Status`: at least one of the related objects matches.
* **Paths can be nested**, e.g. `Orders.Items.Product.Category` for the customers that bought a product of a category.

The conditions of one map whose paths start with the same relation field are about **the same related object**. Conditions in different maps can match different related objects:

```go
// customers with an order that is paid and over 100
sqlfilters.Filters{
    "Orders.Status": {Op: sqlfilters.OpEqual, Val: "paid"},
    "Orders.Total":  {Op: sqlfilters.OpGreater, Val: 100},
}

// customers with a paid order, and an order over 100, which can be another order
sqlfilters.And(
    sqlfilters.Filters{"Orders.Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
    sqlfilters.Filters{"Orders.Total": {Op: sqlfilters.OpGreater, Val: 100}},
)
```

The conditions are `EXISTS` subqueries, not joins, so an object is returned once, however many related objects match, and `Limit` and `Offset` count the objects. They work in `Get`, `GetCount`, `UpdateMultiple` and `DeleteMultiple`, also in groups with `Or`.

* **Values** are converted from strings and validated with the tags of the related struct, and the violations are named by the path, e.g. `{"Customer.Country": "len"}`.
* **A path through a field that is not a relation field**, e.g. `Status.Name`, is an error that matches `gocrud.ErrFilterPath`.
* **Indexes.** The subqueries find the related objects by their key field, so index it, e.g. `crud:"idx fk:customer"`. PostgreSQL does not create indexes for foreign keys by itself.
* **Own select queries.** The subqueries refer to the table by its name, so `Get` and `GetCount` cannot use them for a struct that builds its own `SELECT` with `BuildSelectPrefixQuery`, e.g. with [column aliases](../struct.md#column-aliases), and return an error instead.

### Subqueries

The value of `OpIn` can be a subquery, which selects the values of a field of the objects of another struct that match a filter. The structs do not need a relation, e.g. the users who have an invitation that is not accepted yet:

```go
users, err := gocrud.Get[User](ctx, crud, gocrud.GetOptions{
    Filters: sqlfilters.Filters{
        "Email": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&Invitation{}, "Email", sqlfilters.Filters{
            "AcceptedAt": {Op: sqlfilters.OpEqual, Val: nil},
        })},
    },
})
```

```sql
WHERE "email" IN (SELECT s1."email" FROM "invitation" s1 WHERE s1."accepted_at" IS NULL)
```

```go
func Subquery(obj interface{}, field string, filter Filter) SubqueryValue
```

* **The filter** of a subquery is any filter of the struct, including groups, [paths](#conditions-on-related-records) and other subqueries, or `nil` for all the objects. It is validated with the tags of the struct, with the violations named by its fields, and its values are not converted from strings. `obj` is used only for its type.
* **Subqueries work** in `Get`, `GetCount`, `UpdateMultiple`, `DeleteMultiple` and the `Filters` of [`Aggregate`](reports.md), also in groups with `Or` and in conditions on related records, e.g. `Customer.ID`.
* **The field** of a subquery has to be stored, and its values have to be of the same kind as the values of the field of the condition, as they are stored: numbers, strings, booleans, times or bytes. A decimal type that is written as text is a string, and a `sql.NullInt64` a number. Fields stored as JSON cannot be compared.
* **A subquery that cannot be used**, e.g. with an operator other than `OpIn`, on a field that is not stored or of another kind, or with an invalid filter, is an error that matches `gocrud.ErrSubquery`, and also `*gocrud.ValidationError` for invalid values in its filter.
* **For related objects** paths are shorter, e.g. `"Customer.Country"` instead of `"CustomerID"` with a subquery of the customers from a country, and they work for has-many relations too.

### Raw SQL

`Raw` is a fragment of SQL, for conditions that the other filters cannot express, e.g. with an SQL function:

```go
users, err := gocrud.Get[User](ctx, crud, gocrud.GetOptions{
    Filters: sqlfilters.And(
        sqlfilters.Filters{"Active": {Op: sqlfilters.OpEqual, Val: true}},
        sqlfilters.Raw("LOWER(.Email) = ? OR .ID IN (?)", strings.ToLower(email), adminIDs),
    ),
})
```

* **Fields** are written as `.FieldName`, and replaced with their columns. A dot after a letter, a digit or a quote is left as it is, e.g. in `t1.price` or `1.5`.
* **Each `?`** is replaced with a parameter of the query for one of the values, in order. A slice or an array is a list of parameters, e.g. for `IN (?)`. The number of `?` has to be the number of values, otherwise the operation fails with an error that matches `gocrud.ErrRawFilter`.
* **The SQL** is used as it is, so never build it from user input. Pass the input as values.

The values of a raw filter are not validated, and not converted from strings.

### Values from strings

With `ConvertFiltersFromString`, `Get` and `GetCount` convert the string values of the conditions on fields to the types of the fields, also in groups. This is how values from HTTP query parameters are used:

* **Times** can be written in several formats, e.g. `2024-05-01T10:30:00Z` or `2024-05-01`, see [Times](../struct.md#3-times).
* **`OpIn`** values are separated with commas, e.g. `"paid,shipped"`, and an empty string is an empty list. A value cannot contain a comma.
* **An empty string** is `NULL` for a nullable field, also of a Valuer type whose zero value is `NULL`, e.g. `sql.NullString`.
* **Valuer types** are converted with their `UnmarshalText` method, or scanned, e.g. a decimal from `"12.50"`, see [Valuer types](../struct.md#7-valuer-types). Bytes and JSON are not converted from strings.
* **A value that cannot be converted** is a validation error with the `type` violation, also a number that does not fit in the type of the field, e.g. `300` for a `uint8`.

### Validation

Each condition on a field is validated as if its value was the value of the field, with the `validate` and `crud_regexp` tags, e.g. `"Status": {Op: sqlfilters.OpEqual, Val: "lost"}` fails for a field with `validate:"oneof=new paid shipped"`. The operation then returns a `*gocrud.ValidationError` with the violations of all the conditions, by field name.

**Password fields** cannot be used in filters, also through relation fields and in subqueries, e.g. `Author.Password`: a condition on a hash would reveal it, e.g. with `OpLike`. Such a condition is the violation `"password"` (`gocrud.ViolationPassword`), e.g. `{"Password": "password"}`. Password fields cannot be used in `Order` either, and a subquery cannot select one. To find e.g. the users without a password, write the column in a [raw filter](#raw-sql), ``sqlfilters.Raw(`"password" = ''`)``, as `.Password` is a password field too.

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

// filters as Get validates them, also on the fields of related structs and in subqueries, returns a *gocrud.CRUDError with a *gocrud.ValidationError
err = crud.ValidateFilters(obj, filters)
```

`violations` maps the names of the invalid fields to the names of the failed rules, see [Validation Errors](../struct.md#validation-errors). The last argument of the functions is the `TagName` of the CRUD, which is used for the `<tagName>_regexp` tags. `err` is set when `obj` cannot be validated, e.g. it is not a pointer to a struct.

## Errors

Most errors are a `*gocrud.CRUDError`, whose `Op` describes the step that failed, e.g. `"database exec"`, and whose `Err` is the cause. `Op` is for the message of the error, and can change in any version, so do not compare it. `UniqError`, and `ErrNotFound` of `Load[T]`, are returned as they are. Use `errors.Is` and `errors.As` to check any of them:

| Error | When |
|---|---|
| `*gocrud.ValidationError` | the object or a filter is not valid, `Violations` has the failed rules |
| `*gocrud.UniqError` | a unique field already has the value, see below |
| `gocrud.ErrForeignKeyViolation` | a foreign key is violated, see [Relations](relations.md#violations) |
| `gocrud.ErrNotFound` | `Load[T]` did not find the object, or `Save` with `NoInsert` did not find the object to update |
| `*gocrud.ObjectError` | one of the objects of [`InsertMultiple`](#insertmultiple) failed, e.g. with a `ValidationError`, see its `Index` |
| `gocrud.ErrInsertMultiple` | `InsertMultiple` of objects of different types |
| `gocrud.ErrVersionConflict` | `Save` of an object whose [version](../struct.md#10-version) is not the stored one, as the object was changed since it was loaded |
| `gocrud.ErrAggregate` | a result struct or options of [`Aggregate`](reports.md) cannot be used |
| `gocrud.ErrCursor` | a cursor that cannot be used, see [Pages with cursors](#pages-with-cursors) |
| `gocrud.ErrInvalidOrder` | `Order` of `Get` has a field that does not exist, a password field, a path through a field that is not a belongs-to relation, or an odd number of elements |
| `gocrud.ErrSubquery` | a filter has a [subquery](#subqueries) that cannot be used |
| `gocrud.ErrFilterPath` | a filter has a path through a field that is not a relation field, see [Conditions on related records](#conditions-on-related-records) |
| `gocrud.ErrPreload` | a name in `Preload` is not a relation field |
| `gocrud.ErrInvalidID` | an ID, or the ID of a user in `SaveOptions.ModifiedBy`, is of the wrong type, e.g. a number for a struct with a UUID, or a string that is not a UUID |
| `gocrud.ErrUnsupportedOption` | an option is not supported by a [typed function](generics.md) |
| `gocrud.ErrQueryMethod` | a [custom query method](custom-queries.md#query-methods) has a signature other than the one gocrud calls |
| `*gocrud.SchemaError`, `gocrud.ErrSchemaDiffers` | `CheckSchema` found tables that do not match the structs, see [Migrations](migrations.md#checking-the-schema) |
| `gocrud.ErrAliasedColumnsMigrate` | `Migrate` of a struct with [column aliases](../struct.md#column-aliases) |
| `gocrud.ErrNotStructPointer`, `ErrIDField`, `ErrNoColumns`, `ErrPasswordField`, `ErrModificationFields`, `ErrColumnType`, `ErrVersionField`, `ErrRelation`, `ErrForeignKey` | the struct cannot be used, see [Struct](../struct.md) |
| `gocrud.ErrRawFilter` | a [raw filter](#raw-sql) has another number of `?` than of values |
| `gocrud.ErrInFilter` | the value of an `OpIn` condition is not a slice, an array or a [subquery](#subqueries) |
| `gocrud.ErrMatchFilter` | `OpMatch` with SQLite, which does not support it |
| `gocrud.ErrPasswordFilter` | a raw filter or `Order` with a password field, e.g. `.Password`, see [Validation](#validation) |

`UniqError` names the fields of the duplicate value, in the same way in PostgreSQL and SQLite, with any driver:

* `Fields` are the fields of the unique field or index, e.g. `[Email]`, or `[Tenant Slug]` for a `uidx` index of two fields,
* `Field` is the first of them, e.g. `Email`.

A unique index or constraint that gocrud did not create, e.g. one created by hand, is not a field of the struct, so `Fields` is empty, and `Field` is how the database names it: the name of the index in PostgreSQL, e.g. `user_note_key`, and the table and the columns in SQLite, e.g. `user.note`.
