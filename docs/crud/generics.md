# Typed functions

The methods of `CRUD` take and return `interface{}`, so the results of `Get` have to be converted to your type, and a mistake shows up only when the program runs. The typed functions do the same work, but take the struct type as a type parameter and return values of that type:

```go
users, err := gocrud.Get[User](ctx, crud, gocrud.GetOptions{
    Order: []string{"Username", "asc"},
})
// users is []*User
```

They are functions and not methods, because methods in Go cannot have type parameters. The type parameter is the struct type, e.g. `Get[User]`, and not a pointer to it.

| Function | Instead of |
|---|---|
| `Get[T](ctx, crud, GetOptions) ([]*T, error)` | `crud.Get(ctx, func() interface{} { return &T{} }, options)` |
| `Load[T](ctx, crud, id uint64) (*T, error)` | `crud.Load(ctx, &T{}, strconv.FormatUint(id, 10), LoadOptions{})` |
| `GetCount[T](ctx, crud, GetCountOptions) (uint64, error)` | `crud.GetCount(ctx, &T{}, options)` |
| `UpdateMultiple[T](ctx, crud, values, UpdateMultipleOptions) error` | `crud.UpdateMultiple(ctx, &T{}, values, options)` |
| `DeleteMultiple[T](ctx, crud, DeleteMultipleOptions) error` | `crud.DeleteMultiple(ctx, &T{}, options)` |
| `CreateTable[T](ctx, crud) error` | `crud.CreateTable(ctx, &T{})` |
| `DropTable[T](ctx, crud) error` | `crud.DropTable(ctx, &T{})` |

`Save` and `Delete` do not have typed functions, because they already take your object, e.g. `crud.Save(ctx, user, gocrud.SaveOptions{})`.

## Get

`Get` returns the matching objects as `[]*T`. When nothing matches, it returns an empty slice, so it is encoded as `[]` in JSON, not as `null`.

```go
func ListActiveUsers(ctx context.Context, crud *gocrud.CRUD, department string) ([]*User, error) {
    return gocrud.Get[User](ctx, crud, gocrud.GetOptions{
        Order: []string{"LastName", "asc", "FirstName", "asc"},
        Limit: 50,
        Filters: &sqlfilters.Filters{
            "Department": {Op: sqlfilters.OpEqual, Val: department},
            "IsActive":   {Op: sqlfilters.OpEqual, Val: true},
        },
    })
}
```

All the options of `GetOptions` work the same way as in [`CRUD.Get`](ops.md#get), except two that do not fit a typed result:

* `RowObjTransformFunc` changes the type of the results.
* `VerifyPasswordFields` returns the result of the verification next to the objects.

`Get` returns an error that matches `gocrud.UnsupportedOptionError` when one of them is set. Use `CRUD.Get` for them.

## Load

`Load` returns the object with the given ID. When there is no such object, it returns `gocrud.NotFoundError`, instead of an empty object:

```go
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
    if err != nil {
        http.Error(w, "invalid id", http.StatusBadRequest)
        return
    }

    user, err := gocrud.Load[User](r.Context(), h.crud, id)
    switch {
    case errors.Is(err, gocrud.NotFoundError):
        http.Error(w, "user not found", http.StatusNotFound)
    case err != nil:
        http.Error(w, "internal error", http.StatusInternalServerError)
    default:
        _ = json.NewEncoder(w).Encode(user)
    }
}
```

`service.NotFoundError` is the same error, so `errors.Is` works with either of them.

Password verification with `LoadOptions.VerifyPasswordFields` is done with `CRUD.Load`.

## Counting, updating and deleting

```go
// how many orders are waiting to be shipped
waiting, err := gocrud.GetCount[Order](ctx, crud, gocrud.GetCountOptions{
    Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
})

// mark all the paid orders of a customer as shipped
err = gocrud.UpdateMultiple[Order](ctx, crud, map[string]interface{}{"Status": "shipped"}, gocrud.UpdateMultipleOptions{
    Filters: &sqlfilters.Filters{
        "CustomerID": {Op: sqlfilters.OpEqual, Val: customerID},
        "Status":     {Op: sqlfilters.OpEqual, Val: "paid"},
    },
})

// remove the items of an order
err = gocrud.DeleteMultiple[OrderItem](ctx, crud, gocrud.DeleteMultipleOptions{
    Filters: &sqlfilters.Filters{"OrderID": {Op: sqlfilters.OpEqual, Val: orderID}},
})
```

## In transactions

The functions take the `*CRUD` to use as an argument, so in a [transaction](transactions.md) pass `tx`:

```go
err := crud.RunInTx(ctx, func(tx *gocrud.CRUD) error {
    order, err := gocrud.Load[Order](ctx, tx, orderID)
    if err != nil {
        return err
    }

    order.Status = "cancelled"
    if err := tx.Save(ctx, order, gocrud.SaveOptions{NoInsert: true}); err != nil {
        return err
    }

    return gocrud.DeleteMultiple[OrderItem](ctx, tx, gocrud.DeleteMultipleOptions{
        Filters: &sqlfilters.Filters{"OrderID": {Op: sqlfilters.OpEqual, Val: order.ID}},
    })
})
```
