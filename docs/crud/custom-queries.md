# Custom queries

gocrud generates the SQL of every operation from the struct. A struct can replace any of these queries with its own, by implementing a method. There are two kinds of methods:

* **Query methods**, e.g. `SelectQuery`, are called on every operation and return the whole query. They can build it from the options of the operation.
* **Builder methods**, e.g. `BuildSelectPrefixQuery`, are called once, when the struct is used for the first time, and get the table name prefix. They replace a query, or the beginning of a query to which gocrud adds the conditions.

An error returned by any of these methods is returned by the operation, and matches with `errors.Is`.

## Example: soft delete

Deleted notes are kept, but hidden:

```go
type Note struct {
    ID     uint64 `json:"id"`
    Text   string `json:"text"`
    Hidden bool   `json:"-"`
}

// Delete hides the note instead of deleting it.
func (n *Note) DeleteByIDQuery() (string, error) {
    return `UPDATE "note" SET "hidden" = true WHERE "id" = $1`, nil
}

// DeleteMultiple hides the notes, the query has to return their IDs.
func (n *Note) DeleteReturningIDQuery() (string, error) {
    return `UPDATE "note" SET "hidden" = true RETURNING "id"`, nil
}

// Get and GetCount do not return the hidden notes.
func (n *Note) SelectQuery(order []string, limit int, offset int, filters sqlfilters.Filter) (string, error) {
    return `SELECT "id","text","hidden" FROM "note" WHERE "hidden" = false ORDER BY "id"`, nil
}

func (n *Note) SelectCountQuery(filters sqlfilters.Filter) (string, error) {
    return `SELECT COUNT(*) FROM "note" WHERE "hidden" = false`, nil
}
```

`Load` still returns hidden notes, because `SelectByIDQuery` is not replaced. The queries above ignore the filters, ordering and pagination, so a real implementation would add them, or use the builder methods below, which keep them.

## Query methods

A method must have exactly the signature in the table. A method with the name of a query method and another signature, e.g. one written for an older version of gocrud, would not be called, so the operations on the struct fail with an error that matches `gocrud.ErrQueryMethod` instead.

The query is run with the same arguments as the generated query, in the order below. "Columns" are the stored fields of the struct, in the order they are declared, i.e. without the ignored fields and the relation fields.

| Method | Used by | Arguments of the query | The query returns |
|---|---|---|---|
| `InsertQuery() (string, error)` | `Save` of a new object | the columns without `ID`, or all the columns for a UUID, which `Save` generates | the new `ID` |
| `InsertOnConflictUpdateQuery() (string, error)` | `Save` of an object with an `ID` | all the columns, then the updated columns, then the version of the object, for a struct with a [version](../struct.md#10-version) | nothing, or the stored version for a struct with a version, and no row when it is another one |
| `UpdateByIDQuery() (string, error)` | `Save` with `NoInsert` | the updated columns, then the `ID`, then the version of the object, for a struct with a version | nothing, and `Save` returns `ErrNotFound`, or `ErrVersionConflict` when the row exists, when it changes no rows |
| `SelectByIDQuery() (string, error)` | `Load` | the `ID` | the columns |
| `SelectQuery(order []string, limit int, offset int, filters sqlfilters.Filter) (string, error)` | `Get` | the filter values | the columns |
| `SelectCountQuery(filters sqlfilters.Filter) (string, error)` | `GetCount` | the filter values | the number of rows |
| `UpdateQuery(values map[string]interface{}, filters sqlfilters.Filter) (string, error)` | `UpdateMultiple` | the values, ordered by field name, then the filter values | nothing |
| `DeleteByIDQuery() (string, error)` | `Delete` | the `ID` | nothing |
| `DeleteReturningIDQuery() (string, error)` | `DeleteMultiple` | the filter values | the IDs of the rows, which `DeleteMultiple` returns |
| `CreateTableQuery() (string, error)` | `CreateTable`, `Migrate` | none, statements can be separated with `;` | nothing |
| `DropTableQuery() (string, error)` | `DropTable` | none | nothing |

* **The updated columns** are the columns without `ID` and the [version](../struct.md#10-version), and without `CreatedAt` and `CreatedBy` when the struct has the [creation and modification fields](../struct.md#4-creation-and-modification-fields). The version is incremented by the query itself, e.g. `"version"="note"."version"+1`.
* **The filter values** are the values that `sqlfilters.FiltersInterfaces(filters)` returns: the values of the filters of a group in their order, and of the conditions in a map ordered by field name, with one value for each element of `OpIn`, the values of the filter of a subquery, none for `IS NULL` conditions, and one for each element of a slice in a raw filter. `sqlfilters.Conditions(filters)` returns the conditions on fields in the same order, e.g. to build the `WHERE` clause.
* **Placeholders** are `$1`, `$2`, … in PostgreSQL and `?` in SQLite. Times are converted for SQLite as in the generated queries.
* **Password columns** get an empty value when the password of an object is not changed, see [`pass`](../struct.md#the-crud-tag). The generated update queries keep the stored password then, e.g. `"password"=COALESCE(NULLIF($2,''),"user"."password")`, and so should your own `UpdateByIDQuery` and `InsertOnConflictUpdateQuery`, as well as `BuildUpdateByIDQuery` and `BuildInsertOnConflictUpdateQuery` below.

## Builder methods

A builder method gets the table name prefix, so that the query can use the right table names. For example, `Get` and `GetCount` can read the notes from a database view of the notes that are not hidden, and still use the filters, ordering and pagination of gocrud:

```go
// CREATE VIEW visible_note AS SELECT * FROM note WHERE hidden = false

func (n *Note) BuildSelectPrefixQuery(tableNamePrefix string) (string, error) {
    return `SELECT "id","text","hidden" FROM "` + tableNamePrefix + `visible_note"`, nil
}

func (n *Note) BuildSelectCountPrefixQuery(tableNamePrefix string) (string, error) {
    return `SELECT COUNT(*) AS cnt FROM "` + tableNamePrefix + `visible_note"`, nil
}
```

These methods replace a whole query:

| Method | Replaces the query of |
|---|---|
| `BuildCreateTableQuery(tableNamePrefix string) (string, error)` | `CreateTable` |
| `BuildDropTableQuery(tableNamePrefix string) (string, error)` | `DropTable` |
| `BuildInsertQuery(tableNamePrefix string) (string, error)` | `Save` of a new object |
| `BuildInsertOnConflictUpdateQuery(tableNamePrefix string) (string, error)` | `Save` of an object with an `ID` |
| `BuildUpdateByIDQuery(tableNamePrefix string) (string, error)` | `Save` with `NoInsert` |
| `BuildSelectByIDQuery(tableNamePrefix string) (string, error)` | `Load` |
| `BuildDeleteByIDQuery(tableNamePrefix string) (string, error)` | `Delete` |

These methods replace the beginning of a query, and gocrud adds the rest, e.g. ` WHERE … ORDER BY … LIMIT …`:

| Method | Replaces | Default |
|---|---|---|
| `BuildSelectPrefixQuery(tableNamePrefix string) (string, error)` | the beginning of `Get` | `SELECT <columns> FROM <table>` |
| `BuildSelectCountPrefixQuery(tableNamePrefix string) (string, error)` | the beginning of `GetCount` | `SELECT COUNT(*) AS cnt FROM <table>` |
| `BuildDeletePrefixQuery(tableNamePrefix string) (string, error)` | the beginning of `DeleteMultiple` | `DELETE FROM <table>` |
| `BuildUpdatePrefixQuery(tableNamePrefix string) (string, error)` | the beginning of `UpdateMultiple` | `UPDATE <table> SET` |

The queries take the same arguments, and return the same results, as the query methods above. A prefix cannot have a `WHERE` clause of its own, because gocrud adds ` WHERE <filters>` to it when there are filters. Use a view, as above, or a query method when a condition has to be combined with the filters.

The select prefixes are the way to read from several tables, with [column aliases](../struct.md#column-aliases).

When a struct has both a query method and a builder method for the same operation, the query method is used.
