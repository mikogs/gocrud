# Helper functions

These functions are used by gocrud itself, and are exported for code that works with structs of any type, e.g. a REST API built on top of the [registry](pkgs/registry.md). Most applications do not need them.

## Objects

In package `gocrud`. `obj` is a pointer to a struct.

| Function | Description |
|---|---|
| `ObjIDValue(obj) interface{}` | returns the `ID`: a `uint64`, or a `string` with a UUID |
| `ObjIDIsZero(obj) bool` | checks if the `ID` is not set, `0` or `""`, as in an object that is not saved |
| `ObjSetIDValue(obj, id interface{}) error` | sets the `ID` to a non-negative integer, or a decimal string, for a `uint64` ID, and to a UUID in its standard form for a `string` ID. `nil` clears the `ID`. Another value returns an error that matches `ErrInvalidID` |

The constant `IDField` is `"ID"`, the name of the ID field.

## Filters

In package `github.com/mikogs/gocrud/pkg/sqlfilters`.

| Function | Description |
|---|---|
| `Subquery(obj, field string, filter Filter) SubqueryValue` | returns a subquery, the value of an `OpIn` condition, see [Subqueries](crud/ops.md#subqueries) |
| `FiltersInterfaces(filter Filter) []interface{}` | returns the values of a filter, in the order of the placeholders in the generated query, see [Custom queries](crud/custom-queries.md#query-methods) |
| `Conditions(filter Filter) []Condition` | returns the conditions on fields of a filter, in the same order, without the raw filters. A `Condition` is the name of a field and its `OpVal` |
| `SetObjFields(obj, values *Filters) error` | sets the fields of `obj` to the values of the filters, converting numbers between types |
| `OpVal.IsNull() bool` | checks if the filter is an `IS NULL` or `IS NOT NULL` condition |
| `OpVal.InValues() ([]interface{}, bool)` | returns the values of an `OpIn` filter |
