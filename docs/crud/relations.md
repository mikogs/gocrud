# Relations

Objects are often related to each other: an order belongs to a customer, and has many items. gocrud can:

* load the related objects of the objects you get, without a query for each object, see [Loading related objects](#loading-related-objects),
* create foreign keys, so that the database keeps the references valid, and deletes or updates the related rows when an object is deleted, see [Foreign keys](#foreign-keys).

gocrud never saves related objects for you, and never loads them unless you ask for it.

## Declaring relations

A field that holds related objects is a pointer to a struct or a slice of structs, tagged with `rel:<key field>`. It is not stored in a column of the table.

```go
type Customer struct {
    ID     uint64   `json:"id"`
    Name   string   `json:"name" validate:"required"`
    Orders []*Order `json:"orders,omitempty" crud:"rel:CustomerID"` // has many
}

type Order struct {
    ID         uint64      `json:"id"`
    CustomerID uint64      `json:"customer_id" crud:"fk:customer on_delete:cascade"`
    Status     string      `json:"status"`
    Customer   *Customer   `json:"customer,omitempty" crud:"rel:CustomerID"` // belongs to
    Items      []OrderItem `json:"items,omitempty" crud:"rel:OrderID"`       // has many
}

type OrderItem struct {
    ID        uint64   `json:"id"`
    OrderID   uint64   `json:"order_id" crud:"fk:order on_delete:cascade"`
    ProductID uint64   `json:"product_id" crud:"fk:product"`
    Quantity  int64    `json:"quantity" validate:"min=1"`
    Product   *Product `json:"product,omitempty" crud:"rel:ProductID"` // belongs to
}

type Product struct {
    ID    uint64 `json:"id"`
    Name  string `json:"name"`
    Price int64  `json:"price"`
}
```

There are two kinds of relations:

| Field | Relation | The key field |
|---|---|---|
| a pointer, e.g. `Customer *Customer` | belongs to | is in this struct, e.g. `Order.CustomerID` |
| a slice, e.g. `Items []OrderItem` or `Items []*OrderItem` | has many | is in the related struct, e.g. `OrderItem.OrderID` |

The key field is of type `uint64`, or `*uint64` when the relation is optional, and holds the `ID` of the related object. The declarations are checked when the struct is used for the first time, and a mistake, e.g. a key field that does not exist, is an error that matches `structinfo.RelationError`.

Use `omitempty` in the `json` tag of relation fields, so that they are not encoded when they are not loaded.

Related objects are not validated when the object that holds them is saved. They are validated when they are saved themselves.

## Loading related objects

The `Preload` option names the relation fields to load:

```go
orders, err := gocrud.Get[Order](ctx, crud, gocrud.GetOptions{
    Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
    Preload: []string{"Customer", "Items"},
})

order, err := gocrud.Load[Order](ctx, crud, orderID, "Customer", "Items")
```

`CRUD.Get` and `CRUD.Load` have the same option in `GetOptions.Preload` and `LoadOptions.Preload`.

Each relation is loaded with **one query**, however many objects there are: the orders above take 3 queries, for orders, customers and items. The query gets the related objects of all the objects at once, with an `IN` condition.

* A belongs-to field is set to the related object, or to `nil` when the key field is `0`, `nil`, or the related object does not exist. Objects with the same key share the same related object.
* A has-many field is set to the related objects, ordered by `ID`. An object without related objects gets an empty slice, so it is encoded as `[]` in JSON.

### Nested relations

A name can be a path through relations, e.g. `Items.Product` loads the items of the orders and the products of the items. A path loads all the relations on it, so `Items` does not have to be named as well:

```go
order, err := gocrud.Load[Order](ctx, crud, orderID, "Customer", "Items.Product")

for _, item := range order.Items {
    fmt.Printf("%d x %s\n", item.Quantity, item.Product.Name)
}
```

Each relation on a path is loaded with one query as well, so this takes 4 queries.

Preload names are checked before any query is run. A name that is not a relation field is an error that matches `gocrud.PreloadError`, and names the path, e.g. `Items.Supplier: Supplier is not a relation field of OrderItem`.

### Things to keep in mind

* **In a transaction**, related objects are loaded in the same transaction, when `Get` or `Load` is called with `tx`.
* **`RowObjTransformFunc`** is called after the related objects are loaded, so it can use them.
* **Large results** are loaded in parts, with up to 1000 keys in a query, as databases limit the number of query parameters.
* **Related objects are not saved** when the object that holds them is saved. Save them yourself, in a [transaction](transactions.md) when they belong together.

## Many-to-many relations

A many-to-many relation, e.g. orders with tags, is made of a struct for the table that joins them. It has a has-many relation from one side, and a belongs-to relation to the other:

```go
type Tag struct {
    ID   uint64 `json:"id"`
    Name string `json:"name" crud:"uniq"`
}

type OrderTag struct {
    ID      uint64 `json:"id"`
    OrderID uint64 `json:"order_id" crud:"uidx:order_tag fk:order on_delete:cascade"`
    TagID   uint64 `json:"tag_id" crud:"uidx:order_tag fk:tag on_delete:cascade"`
    Tag     *Tag   `json:"tag,omitempty" crud:"rel:TagID"`
}

// in Order:
//     Tags []OrderTag `json:"tags,omitempty" crud:"rel:OrderID"`

order, err := gocrud.Load[Order](ctx, crud, orderID, "Tags.Tag")
```

The `uidx:order_tag` index makes sure that a tag is added to an order once. The joining struct is a normal struct, so it can have more fields, e.g. when the tag was added.

## Foreign keys

A foreign key is declared on the key field, with `fk:<table>`, and the action on delete with `on_delete:<action>`:

```go
type Order struct {
    ID         uint64  `json:"id"`
    CustomerID uint64  `json:"customer_id" crud:"idx fk:customer on_delete:cascade"`
    ShipperID  *uint64 `json:"shipper_id" crud:"fk:shipper on_delete:set_null"`
}
```

`CreateTable` and `Migrate` create the column with `REFERENCES "customer"("id") ON DELETE CASCADE`. The table name is the name of the referenced table, without the `TableNamePrefix`, which is added to it. The key field is of type `uint64`, or `*uint64` when the reference is optional.

| `on_delete` | When the referenced row is deleted |
|---|---|
| `cascade` | the rows that reference it are deleted as well, e.g. the items of a deleted order |
| `set_null` | the key field of the rows that reference it is set to `NULL`, needs a `*uint64` field |
| `restrict` | the delete fails while there are rows that reference it |
| not set | the same as `restrict`, the delete fails (SQL `NO ACTION`) |

The database applies the actions, also to rows deleted with `DeleteMultiple`, by other applications, or with your own SQL.

A mistake in the tags, e.g. `on_delete:set_null` on a field that cannot be `NULL`, is an error that matches `structinfo.ForeignKeyError`.

### Violations

Saving an object that references a row that does not exist, or deleting a row that is referenced with `restrict`, fails with an error that matches `gocrud.ForeignKeyViolationError`:

```go
err := crud.Save(ctx, order, gocrud.SaveOptions{})
switch {
case errors.Is(err, gocrud.ForeignKeyViolationError):
    http.Error(w, "the customer does not exist", http.StatusConflict)
case err != nil:
    http.Error(w, "internal error", http.StatusInternalServerError)
}
```

A key field of type `uint64` is `0` when it is not set, and `0` does not reference any row, so saving such an object fails too. Use `*uint64` for references that are optional.

### SQLite

SQLite checks foreign keys only when they are enabled for the database connection. Enable them in the connection string:

```go
db, err := sql.Open("sqlite", "file:app.db?_pragma=foreign_keys(1)")
```

Without it, foreign keys are created, but ignored. `Migrate` reports it in the differences of every struct with foreign keys.

### Creating and migrating tables

A table can be created only after the tables it references. The [service](../pkgs/service.md) `CreateTables` and `Migrate` create and migrate the tables in that order. To order the tables yourself, `crud.TableName(obj)` returns the name of the table of a struct, and `crud.ReferencedTables(obj)` the names of the tables its foreign keys reference. A table that references itself, e.g. a category with a parent category, is fine. Tables that reference each other in a cycle cannot be created one after another, and are an error that matches `service.ForeignKeyCycleError`.

`Migrate` adds new foreign key columns with their foreign keys. In a table that already has rows, a new foreign key column has to be a pointer, so that the existing rows get `NULL`: a column that cannot be `NULL` would get `0`, which does not reference any row, and the migration fails.

A foreign key of an existing column is not added or changed by `Migrate`, because the existing rows may not satisfy it. It is reported in the differences instead, and can be added after the data is checked, e.g. in PostgreSQL:

```sql
ALTER TABLE "order" ADD FOREIGN KEY (customer_id) REFERENCES customer (id) ON DELETE CASCADE;
```

SQLite cannot add a foreign key to an existing column, the table has to be created again, see [Migrations](migrations.md#changes-that-are-not-made).
