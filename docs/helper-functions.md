# Helper functions

These functions are used by gocrud itself, and are exported for code that works with structs of any type, e.g. a REST API built on top of the [service](pkgs/service.md). Most applications do not need them.

## Objects

In package `gocrud`. `obj` is a pointer to a struct.

| Function | Description |
|---|---|
| `ObjIDValue(obj) uint64` | returns the `ID` |
| `ObjSetIDValue(obj, id uint64)` | sets the `ID` |
| `ObjIDInterface(obj) interface{}` | returns a pointer to the `ID` field, e.g. for `Scan` |
| `ObjFieldValue(obj, fieldName string) interface{}` | returns the value of a field: `int64` for integers, `uint64` for unsigned integers, `float64`, `bool`, `string` or `time.Time`, the value a pointer field points to, and `nil` for `NULL` or a field that does not exist |
| `ObjFieldInterfaces(obj, includeID bool) []interface{}` | returns pointers to the fields of the supported types, in the order of the struct. Fields with the `-` tag are included, as tags are not read |
| `ZeroObjFields(obj)` | sets all the exported fields to their zero values |
| `SetObjCreated(obj, at time.Time, by uint64)` | sets `CreatedAt` and `CreatedBy` |
| `SetObjModified(obj, at time.Time, by uint64)` | sets `ModifiedAt` and `ModifiedBy` |
| `SetObjStringField(obj, fieldName, value string)` | sets a field of type `string` |

The constant `IDField` is `"ID"`, the name of the ID field.

## Values from strings

In package `github.com/mikogs/gocrud/pkg/sql/builder`.

| Function | Description |
|---|---|
| `StructFieldValueFromString(obj, name, value string) (bool, interface{})` | converts a string to the type of a field, as `ConvertFiltersFromString` does, and returns false when it cannot be converted |
| `SetStructFieldValueFromString(obj, name, value string) (bool, interface{})` | converts a string and sets the field to it, e.g. from a form. An empty string sets a pointer field to `nil` |
| `TimeFromString(value string) (time.Time, bool)` | converts a string in one of the [time formats](struct.md#3-times) |
| `IsStructField(obj, field string) bool` | checks if the struct has an exported field of a supported type with the name |
| `MapInterfaces(values map[string]interface{}) []interface{}` | returns the values of a map ordered by the keys, as `UpdateMultiple` passes them to the query |
| `PrettifyCreateTable(sql string) string` | puts the columns of a `CREATE TABLE` statement in separate lines |

## Filters

In package `github.com/mikogs/gocrud/pkg/filters`.

| Function | Description |
|---|---|
| `FiltersInterfaces(filters *Filters) []interface{}` | returns the values of the filters, in the order of the placeholders in the generated query, see [Custom queries](crud/custom-queries.md#query-methods) |
| `SetObjFields(obj, values *Filters) error` | sets the fields of `obj` to the values of the filters, converting numbers between types |
| `OpVal.IsNull() bool` | checks if the filter is an `IS NULL` or `IS NOT NULL` condition |
| `OpVal.InValues() ([]interface{}, bool)` | returns the values of an `OpIn` filter |

## Logging

Package `github.com/mikogs/gocrud/pkg/logger` has helpers for `log/slog`, used by the service:

| Function | Description |
|---|---|
| `SetLogger(level string)` | sets the default logger to a text logger on stderr, with the level `err`, `warn`, `debug`, or info for any other value |
| `AttrError(err)`, `AttrPath(path)` | return the `err` and `path` attributes |
| `AttrModule(obj)`, `AttrHandler(obj)`, `AttrService(obj, action)` | return the `mod`, `hand` and `svc` attributes with the type of `obj` |
| `LogError(msg string, attrs ...interface{}) string` | logs an error with a unique code in the `err_code` attribute, and returns the code, e.g. to show it to the user |
