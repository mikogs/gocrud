# Reports

`Aggregate` returns aggregated values of objects, e.g. their number and sums, grouped by fields, such as the sales by status, or by the country of the customer and month:

```go
type SalesByCountry struct {
    Country string    `json:"country" crud:"group:Customer.Country"`
    Month   time.Time `json:"month" crud:"group:PlacedAt:month"`
    Orders  int64     `json:"orders" crud:"agg:count"`
    Revenue int64     `json:"revenue" crud:"agg:sum:Total"`
    Average float64   `json:"average" crud:"agg:avg:Total"`
}

sales, err := gocrud.Aggregate[Order, SalesByCountry](ctx, crud, gocrud.AggregateOptions{
    Filters: sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
    Having:  sqlfilters.Filters{"Revenue": {Op: sqlfilters.OpGreaterOrEqual, Val: 1000}},
    Order:   []string{"Month", "asc", "Revenue", "desc"},
})
// sales is []*SalesByCountry
```

```go
func Aggregate[T any, R any](ctx context.Context, c *CRUD, options AggregateOptions) ([]*R, error)
```

`T` is the struct of the objects, e.g. `Order`, and `R` the result struct, which describes the report. The result struct is not a table, so it does not need an `ID`.

## The result struct

Each exported field of the result struct has one of these tags:

| Tag | The value |
|---|---|
| `group:<field>` | the value of a field, by which the objects are grouped, e.g. `group:Status` |
| `group:<path>` | the value of a field of a related object, through belongs-to [relations](relations.md), e.g. `group:Customer.Country` |
| `group:<field>:<unit>` | a time truncated to a `day`, `month` or `year`, e.g. `group:PlacedAt:month` |
| `agg:count` | the number of objects |
| `agg:count:<field>` | the number of objects in which the field is not `NULL` |
| `agg:sum:<field>`, `agg:avg:<field>` | the sum and the average of a field |
| `agg:min:<field>`, `agg:max:<field>` | the smallest and the largest value of a field, also of times |

The objects are grouped by all the group fields together. Without group fields, the result is one object with the values of all the objects, e.g. the number of the orders and their total.

* **Types.** Use `int64` for counts, the type of the field or `int64` for sums, `float64` for averages, the type of the field for minimums, maximums and group fields, and `time.Time` for truncated times. [Valuer types](../struct.md#7-valuer-types) can be used too, e.g. a decimal type for the sum of a decimal field, or `sql.NullTime` for a truncated time or the last time.
* **Times** are truncated in UTC, e.g. an order placed on 1 July at 01:00 in Warsaw, which is 30 June at 23:00 in UTC, is in June. The results are in UTC.
* **Related objects** are joined with a `LEFT JOIN`, so the objects without a related object are in a group of their own, e.g. the orders without a customer.
* **NULL.** A field that is not a pointer gets the zero value instead of `NULL`, e.g. `""` for the country of the orders without a customer, `0` for the sum of values that are all `NULL`, and `0.00` for a decimal type that writes its zero value so. Use a pointer, e.g. `*string`, to tell them apart: it is `nil` for `NULL`. A Valuer type whose zero value is `NULL`, e.g. `sql.NullInt64`, gets `NULL` as it is. A time has no zero value in SQL, so a `NULL` time needs a `*time.Time` or `sql.NullTime` field.

## Options

```go
type AggregateOptions struct {
    Filters sqlfilters.Filter // the objects that are aggregated, as in Get
    Having  sqlfilters.Filter // the results, by the fields of the result struct
    Order   []string          // fields of the result struct and directions
    Limit   int
    Offset  int
}
```

* **Filters** are the same [filters](ops.md#filters) as in `Get`, including the [conditions on related records](ops.md#conditions-on-related-records) and [subqueries](ops.md#subqueries), and they are validated in the same way.
* **Having** selects the results by their aggregated values, e.g. the groups with a revenue of at least 1000. Its fields are the fields of the result struct, and it cannot have subqueries.
* **Order**, **Limit** and **Offset** sort and paginate the results, by the fields of the result struct, e.g. the 10 countries with the highest revenue.

## Errors

A result struct or options that cannot be used, e.g. a field without a tag, an unknown function or unit, a field that does not exist, a path through a field that is not a belongs-to relation, or `Having` and `Order` on a field that is not in the result struct, are an error that matches `gocrud.ErrAggregate`. The error names the field, e.g. `field Month: unknown unit "week", want day, month or year`.

## Things to keep in mind

* **Only fields of the struct are aggregated**, e.g. `agg:sum:Total`. The fields of related objects can be group fields, but not aggregated, as a has-many relation would count the objects many times.
* **Password fields** cannot be group fields or aggregated, as the results would have their hashes.
* **Indexes.** Grouping by a field and filtering on it is faster with an index, e.g. `crud:"idx"` on `Status`.
* **Decimals in SQLite.** The aggregated values have no column type, so in SQLite `Having` compares a value of a decimal type, which is written as text, e.g. `"100.00"`, as text with the number of a sum, and never matches. Use a number, e.g. `100.0`, in `Having`, or PostgreSQL, which converts it.
* **Reports that do not fit**, e.g. with window functions or several levels of grouping, are better written as SQL, and run on the database, in a [transaction](transactions.md) with `tx.Tx()` when needed.
