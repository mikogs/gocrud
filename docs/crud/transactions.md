# Transactions

When several changes belong together, they should either all be saved or none of them. Placing an order is a good example: the order, its items and the stock of every product have to be updated together. If any step fails halfway through, you do not want an order without items, or stock taken for an order that was never placed.

gocrud runs such changes in a database transaction with `RunInTx`.

## RunInTx

```go
func (c *CRUD) RunInTx(ctx context.Context, fn func(tx *CRUD) error) error
```

`RunInTx` starts a transaction and calls `fn` with `tx`, a `*CRUD` that runs all its queries in that transaction. It has the same methods as the original `CRUD`.

* When `fn` returns `nil`, the transaction is **committed**.
* When `fn` returns an error, the transaction is **rolled back** and `RunInTx` returns that same error, so you can still check it with `errors.Is` and `errors.As`.
* When `fn` panics, the transaction is **rolled back** and the panic is passed on.

```go
err := crud.RunInTx(ctx, func(tx *gocrud.CRUD) error {
    if err := tx.Save(ctx, user, gocrud.SaveOptions{}); err != nil {
        return err
    }

    profile := &Profile{UserID: user.ID, Language: "en"}
    return tx.Save(ctx, profile, gocrud.SaveOptions{})
})
```

Inside `fn`, always use `tx`, never the original `crud`. Queries run with `crud` are not part of the transaction, see [Things to keep in mind](#things-to-keep-in-mind).

## Example: placing an order

The models:

```go
type Product struct {
    ID    uint64 `json:"id"`
    Name  string `json:"name" validate:"required"`
    Price int64  `json:"price"` // in cents
    Stock int64  `json:"stock" validate:"min=0"`
}

type Order struct {
    ID         uint64 `json:"id"`
    CustomerID uint64 `json:"customer_id" crud:"idx"`
    Status     string `json:"status" validate:"required,oneof=placed paid shipped cancelled"`
    Total      int64  `json:"total"`
}

type OrderItem struct {
    ID        uint64 `json:"id"`
    OrderID   uint64 `json:"order_id" crud:"idx"`
    ProductID uint64 `json:"product_id"`
    Quantity  int64  `json:"quantity" validate:"min=1"`
    Price     int64  `json:"price"` // price at the time of the order
}

type CartItem struct {
    ProductID uint64
    Quantity  int64
}
```

Placing an order saves the order first, because its items need its `ID`. Then, for every product in the cart, it takes the stock and saves an order item. Finally, it saves the order again with the calculated total.

```go
var (
    ErrProductNotFound = errors.New("product not found")
    ErrOutOfStock      = errors.New("product out of stock")
)

func PlaceOrder(ctx context.Context, crud *gocrud.CRUD, customerID uint64, cart []CartItem) (*Order, error) {
    order := &Order{CustomerID: customerID, Status: "placed"}

    err := crud.RunInTx(ctx, func(tx *gocrud.CRUD) error {
        if err := tx.Save(ctx, order, gocrud.SaveOptions{}); err != nil {
            return err
        }

        for _, item := range cart {
            product, err := gocrud.Load[Product](ctx, tx, item.ProductID)
            if errors.Is(err, gocrud.ErrNotFound) {
                return fmt.Errorf("%w: %d", ErrProductNotFound, item.ProductID)
            }
            if err != nil {
                return err
            }

            // Take the stock only if there is enough of it, in a single query.
            res, err := tx.Tx().ExecContext(ctx,
                `UPDATE product SET stock = stock - $1 WHERE id = $2 AND stock >= $1`,
                item.Quantity, product.ID)
            if err != nil {
                return err
            }
            if n, _ := res.RowsAffected(); n == 0 {
                return fmt.Errorf("%w: %s", ErrOutOfStock, product.Name)
            }

            orderItem := &OrderItem{
                OrderID:   order.ID,
                ProductID: product.ID,
                Quantity:  item.Quantity,
                Price:     product.Price,
            }
            if err := tx.Save(ctx, orderItem, gocrud.SaveOptions{}); err != nil {
                return err
            }

            order.Total += product.Price * item.Quantity
        }

        return tx.Save(ctx, order, gocrud.SaveOptions{NoInsert: true})
    })
    if err != nil {
        return nil, err
    }

    return order, nil
}
```

If the second product in the cart is out of stock, `ErrOutOfStock` is returned and everything done so far is rolled back: the order and the first order item are not saved, and the stock of the first product is restored.

Why is the stock taken with a single `UPDATE` instead of checking `product.Stock` and saving the product with a lower value? Two customers can place an order at the same time. Both would read the same stock, both would see that there is enough of it, and both would save it, selling the last item twice. The `UPDATE ... WHERE stock >= $1` checks and takes the stock in one step, and the database makes the second order wait for the first one to finish.

The errors can be turned into the right HTTP responses:

```go
order, err := PlaceOrder(r.Context(), crud, customerID, cart)

var validationErr *gocrud.ValidationError
switch {
case errors.Is(err, ErrProductNotFound):
    http.Error(w, err.Error(), http.StatusNotFound)
case errors.Is(err, ErrOutOfStock):
    http.Error(w, err.Error(), http.StatusConflict)
case errors.As(err, &validationErr):
    http.Error(w, fmt.Sprintf("invalid fields: %v", validationErr.Violations), http.StatusUnprocessableEntity)
case err != nil:
    http.Error(w, "internal error", http.StatusInternalServerError)
default:
    w.WriteHeader(http.StatusCreated)
    _ = json.NewEncoder(w).Encode(order)
}
```

## Running your own SQL

`tx.Tx()` returns the `*sql.Tx` that `tx` runs its queries in, so your own SQL can be part of the same transaction, like the `UPDATE` in the example above. Outside a transaction, `Tx()` returns `nil`.

Note that placeholders depend on the database: `$1`, `$2`, … for PostgreSQL. SQLite accepts both `?` and `$1`.

## Nested calls

When `RunInTx` is called on a `CRUD` that already runs in a transaction, `fn` joins that transaction instead of starting a new one. This lets you write functions that are safe on their own and can also be part of a bigger transaction:

```go
// ReserveStock takes the stock of a product, and fails when there is not enough of it.
func ReserveStock(ctx context.Context, crud *gocrud.CRUD, productID uint64, quantity int64) error {
    return crud.RunInTx(ctx, func(tx *gocrud.CRUD) error {
        res, err := tx.Tx().ExecContext(ctx,
            `UPDATE product SET stock = stock - $1 WHERE id = $2 AND stock >= $1`,
            quantity, productID)
        if err != nil {
            return err
        }
        if n, _ := res.RowsAffected(); n == 0 {
            return ErrOutOfStock
        }
        return nil
    })
}
```

Called with `crud`, `ReserveStock` runs in its own transaction. Called with `tx` from inside `PlaceOrder`, it becomes part of the order's transaction, and its changes are rolled back if the order fails later on:

```go
if err := ReserveStock(ctx, tx, product.ID, item.Quantity); err != nil {
    return err
}
```

An error returned by a nested call does not roll anything back by itself. It is the outer `fn` that decides: when it returns the error, the whole transaction is rolled back.

## Transaction options

`RunInTxWithOptions` takes `*sql.TxOptions` to set the isolation level or make the transaction read-only:

```go
func (c *CRUD) RunInTxWithOptions(ctx context.Context, opts *sql.TxOptions, fn func(tx *CRUD) error) error
```

A sensible use is a report that runs several queries. By default in PostgreSQL, each query sees the data as it is when that query starts. An order paid between two queries would be counted by one of them but not by the other. With `sql.LevelRepeatableRead`, all the queries see the same snapshot:

```go
type SalesReport struct {
    PaidOrders   uint64
    LatestOrders []*Order
}

func GetSalesReport(ctx context.Context, crud *gocrud.CRUD) (*SalesReport, error) {
    report := &SalesReport{}
    paid := &sqlfilters.Filters{
        "Status": {Op: sqlfilters.OpEqual, Val: "paid"},
    }

    opts := &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
    err := crud.RunInTxWithOptions(ctx, opts, func(tx *gocrud.CRUD) error {
        var err error
        report.PaidOrders, err = gocrud.GetCount[Order](ctx, tx, gocrud.GetCountOptions{Filters: paid})
        if err != nil {
            return err
        }

        report.LatestOrders, err = gocrud.Get[Order](ctx, tx, gocrud.GetOptions{
            Filters: paid,
            Order:   []string{"ID", "desc"},
            Limit:   10,
        })
        return err
    })
    if err != nil {
        return nil, err
    }

    return report, nil
}
```

`ReadOnly: true` makes PostgreSQL reject any change made in the transaction.

In SQLite, transactions are always serializable, which is the strictest level, so the isolation level makes no difference. The SQLite driver also **does not enforce** `ReadOnly`, so do not rely on it there.

The options are ignored when `fn` joins an existing transaction.

## Using your own transaction

If you start the transaction yourself, for example because other code shares it, `WithTx` returns a `CRUD` that runs its queries in it. Committing or rolling back is then up to you. All the queries of a `CRUD` are run on a `gocrud.DBTX`, the interface of `*sql.DB`, `*sql.Tx` and `*sql.Conn`.

A common case is the "transactional outbox": an event about a change is saved in the same transaction as the change, so a separate worker can publish it later. The event is never lost, and never published for a change that was rolled back:

```go
func CancelOrder(ctx context.Context, db *sql.DB, crud *gocrud.CRUD, order *Order) error {
    tx, err := db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback() // does nothing after a successful Commit

    order.Status = "cancelled"
    if err := crud.WithTx(tx).Save(ctx, order, gocrud.SaveOptions{NoInsert: true}); err != nil {
        return err
    }

    payload, err := json.Marshal(map[string]uint64{"order_id": order.ID})
    if err != nil {
        return err
    }
    _, err = tx.ExecContext(ctx, `INSERT INTO outbox (topic, payload) VALUES ($1, $2)`, "order.cancelled", payload)
    if err != nil {
        return err
    }

    return tx.Commit()
}
```

## Registry

The [registry](../pkgs/registry.md) package has the same `RunInTx`, `RunInTxWithOptions`, `WithTx` and `Tx` methods. The `tx` passed to `fn` is a `*registry.CRUD` with the same registry.

For example, an HTTP handler that moves a task to another project and records it in the task's activity log:

```go
func (h *Handler) MoveTask(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    taskID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
    projectID, _ := strconv.ParseUint(r.FormValue("project_id"), 10, 64)
    userID := currentUserID(r)

    err := h.reg.RunInTx(ctx, func(tx *registry.CRUD) error {
        obj, _, err := tx.Read(ctx, "tasks", taskID, nil, nil)
        if err != nil {
            return err
        }

        task := obj.(*Task)
        fromProjectID := task.ProjectID
        task.ProjectID = projectID
        if err := tx.Save(ctx, task, userID); err != nil {
            return err
        }

        entry := &TaskActivity{
            TaskID:  task.ID,
            Message: fmt.Sprintf("moved from project %d to project %d", fromProjectID, projectID),
        }
        return tx.Save(ctx, entry, userID)
    })

    var validationErr *registry.ModelValidationError
    switch {
    case errors.Is(err, registry.ErrNotFound):
        http.Error(w, "task not found", http.StatusNotFound)
    case errors.As(err, &validationErr):
        http.Error(w, fmt.Sprintf("invalid fields: %v", validationErr.Violations), http.StatusUnprocessableEntity)
    case err != nil:
        http.Error(w, "internal error", http.StatusInternalServerError)
    default:
        w.WriteHeader(http.StatusNoContent)
    }
}
```

## Things to keep in mind

* **Use `tx` inside `fn`, not `crud`.** Queries run with `crud` are outside the transaction: they do not see its changes until it is committed, and they are not rolled back with it. With SQLite and `db.SetMaxOpenConns(1)`, they wait for the transaction forever.
* **Do not use `tx` after `fn` returns.** The transaction is finished by then, and every query returns an error.
* **Keep transactions short.** A transaction holds locks on the rows it changed until it ends. Do slow work, such as calling other services or sending emails, before or after it.
* **Cancelling `ctx` rolls the transaction back.** This happens when a client disconnects from an HTTP handler that uses `r.Context()`, which is usually what you want.
* **`GetCountOnUniq` works in transactions.** The uniqueness check runs in the transaction, so it also sees the rows saved earlier in it.
* **SQLite allows one writer at a time.** A second transaction that writes waits for the first one, and fails with "database is locked" if it waits too long. With an in-memory database (`:memory:`), every connection gets its own empty database, so call `db.SetMaxOpenConns(1)` to make all the queries use the same one, and keep to the first rule above.
