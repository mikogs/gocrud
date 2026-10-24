package gocrud

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/mikogs/gocrud/internal/dialect"
	"github.com/mikogs/gocrud/internal/sqlbuilder"
	"github.com/mikogs/gocrud/internal/sqlbuilder/where"
	"github.com/mikogs/gocrud/internal/structinfo"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

// ErrAggregate is matched by the error of Aggregate with a result struct or options that cannot be used.
var ErrAggregate = errors.New("invalid aggregate")

// AggregateOptions are the options of Aggregate.
type AggregateOptions struct {
	// Filters select the objects that are aggregated, as in Get.
	Filters sqlfilters.Filter
	// Having selects the results, by the fields of the result struct, e.g. Revenue > 1000.
	Having sqlfilters.Filter
	// Order is a list of fields of the result struct and directions, e.g. []string{"Revenue", "desc"}.
	Order  []string
	Limit  int
	Offset int
}

// aggregateField is a field of a result struct of Aggregate.
type aggregateField struct {
	name string
	// expression is the SQL of the value, e.g. SUM("order"."total"), and group is set for a field of GROUP BY.
	expression string
	group      bool
	// isTime is set for a field of a time, e.g. time.Time or sql.NullTime, or a pointer to one, which SQLite returns as
	// text.
	isTime bool
}

// Aggregate returns the aggregated values of the objects of type T, e.g. their number and sums, grouped by fields, as
// objects of the result struct R. A field of R is tagged with one of:
//
//   - group:<field>, e.g. group:Status, or a path through belongs-to relations, e.g. group:Customer.Country, which
//     groups the objects by the value of the field,
//   - group:<field>:<unit>, e.g. group:CreatedAt:month, which groups them by the time truncated to a day, a month or
//     a year,
//   - agg:count, the number of the objects,
//   - agg:count:<field>, the number of the objects in which the field is not NULL,
//   - agg:sum:<field>, agg:avg:<field>, agg:min:<field> or agg:max:<field>.
//
// Without group fields, the result is one object with the values of all the objects. A field that is not a pointer
// gets the zero value instead of NULL, e.g. the sum of the values that are all NULL, except for times.
func Aggregate[T any, R any](ctx context.Context, c *CRUD, options AggregateOptions) ([]*R, error) {
	obj := new(T)
	builder, err := c.builder(obj)
	if err != nil {
		return nil, getBuilderObjectCRUDError(err)
	}
	ctx = c.withQuery(ctx, "Aggregate", builder.TableName())

	filter, err := c.prepareFilters(obj, options.Filters, false)
	if err != nil {
		return nil, err
	}

	query, args, fields, err := c.aggregateQuery(builder, reflect.TypeOf(obj).Elem(), reflect.TypeOf(new(R)).Elem(), filter, options)
	if err != nil {
		return nil, getCRUDFuncCRUDError("aggregate", err)
	}

	rows, err := c.conn.QueryContext(ctx, query, c.args(args)...)
	if err != nil {
		return nil, getDBFuncCRUDError("query", err)
	}
	defer func() { _ = rows.Close() }()

	results := []*R{}
	for rows.Next() {
		result := new(R)
		if err := scanAggregateRow(rows, reflect.ValueOf(result).Elem(), fields, c.sqlDialect.TimeFormat()); err != nil {
			return nil, getDBFuncCRUDError("rows scan", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("rows", err)
	}

	return results, nil
}

// aggregateQuery returns the query of Aggregate, its arguments, and the fields of the result struct in the order of the
// selected columns.
func (c *CRUD) aggregateQuery(builder *sqlbuilder.Builder, objType reflect.Type, resultType reflect.Type, filter sqlfilters.Filter, options AggregateOptions) (string, []interface{}, []aggregateField, error) {
	if resultType.Kind() != reflect.Struct {
		return "", nil, nil, fmt.Errorf("%w: the result must be a struct, got %s", ErrAggregate, resultType)
	}

	valueNum := 0
	placeholder := func() string {
		valueNum++
		return c.sqlDialect.Placeholder(valueNum)
	}

	table := builder.QuotedTableName()
	dialect := builder.WhereDialect(table, placeholder)
	joins := &aggregateJoins{objType: objType, aliases: map[string]string{}, dialects: map[string]where.Dialect{"": dialect}}

	var fields []aggregateField
	byName := map[string]aggregateField{}
	for i := 0; i < resultType.NumField(); i++ {
		structField := resultType.Field(i)
		if !structField.IsExported() {
			continue
		}

		field, err := c.aggregateField(builder, joins, structField)
		if err != nil {
			return "", nil, nil, fmt.Errorf("%w: field %s: %w", ErrAggregate, structField.Name, err)
		}
		fields = append(fields, field)
		byName[field.name] = field
	}
	if len(fields) == 0 {
		return "", nil, nil, fmt.Errorf("%w: the result struct %s has no fields", ErrAggregate, resultType.Name())
	}

	var selects, groups []string
	for _, field := range fields {
		selects = append(selects, field.expression+` AS "`+field.name+`"`)
		if field.group {
			groups = append(groups, field.expression)
		}
	}

	query := "SELECT " + strings.Join(selects, ",") + " FROM " + table + joins.sql.String()

	conditions, err := where.Build(filter, dialect)
	if err != nil {
		return "", nil, nil, err
	}
	if conditions != "" {
		query += " WHERE " + conditions
	}

	if len(groups) > 0 {
		query += " GROUP BY " + strings.Join(groups, ",")
	}

	having, err := where.Build(options.Having, havingDialect(byName, placeholder, c.sqlDialect))
	if err != nil {
		return "", nil, nil, fmt.Errorf("having: %w", err)
	}
	if having != "" {
		query += " HAVING " + having
	}

	order, err := aggregateOrder(options.Order, byName)
	if err != nil {
		return "", nil, nil, err
	}
	if order != "" {
		query += " ORDER BY " + order
	}

	if options.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", options.Limit)
		if options.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", options.Offset)
		}
	}

	args := append(sqlfilters.FiltersInterfaces(filter), sqlfilters.FiltersInterfaces(options.Having)...)
	return query, args, fields, nil
}

// havingDialect returns the dialect of the conditions of Having, on the fields of the result struct.
// The aggregated values can be of any type, so they are cast to text for LIKE in a database that needs it.
func havingDialect(fields map[string]aggregateField, placeholder func() string, sqlDialect dialect.Dialect) where.Dialect {
	return where.Dialect{
		Column: func(name string) (string, error) {
			field, ok := fields[name]
			if !ok {
				return "", fmt.Errorf("%w: %s is not a field of the result struct", ErrAggregate, name)
			}
			return field.expression, nil
		},
		TextColumn:    func(_ string, column string) string { return sqlDialect.TextColumn(column, true) },
		Placeholder:   placeholder,
		MatchOperator: sqlDialect.MatchOperator(),
	}
}

func aggregateOrder(order []string, fields map[string]aggregateField) (string, error) {
	if len(order)%2 != 0 {
		return "", fmt.Errorf("%w: order has %d elements, want pairs of a field and a direction", ErrAggregate, len(order))
	}

	var parts []string
	for i := 0; i < len(order); i += 2 {
		if _, ok := fields[order[i]]; !ok {
			return "", fmt.Errorf("%w: order: %s is not a field of the result struct", ErrAggregate, order[i])
		}

		direction := "ASC"
		if strings.EqualFold(order[i+1], "desc") {
			direction = "DESC"
		}
		parts = append(parts, `"`+order[i]+`" `+direction)
	}

	return strings.Join(parts, ","), nil
}

// aggregateJoins are the LEFT JOINs of the related tables of the group fields, one for each path of relations.
type aggregateJoins struct {
	objType  reflect.Type
	sql      strings.Builder
	aliases  map[string]string
	dialects map[string]where.Dialect
}

// aggregateField returns the field of a result struct, with the expression of its tag.
func (c *CRUD) aggregateField(builder *sqlbuilder.Builder, joins *aggregateJoins, structField reflect.StructField) (aggregateField, error) {
	field := aggregateField{name: structField.Name, isTime: structinfo.ColumnGoType(structField.Type) == "time.Time"}

	tag := strings.TrimSpace(structField.Tag.Get(c.tagName))
	kind, spec, _ := strings.Cut(tag, ":")
	if strings.Contains(tag, " ") || spec == "" && kind != "agg" {
		return field, fmt.Errorf("the %s tag must be group:<field> or agg:<function>, got %q", c.tagName, tag)
	}

	switch kind {
	case "group":
		path, unit, hasUnit := strings.Cut(spec, ":")
		column, sourceType, err := c.aggregateColumn(builder, joins, path)
		if err != nil {
			return field, err
		}
		if hasUnit {
			if structinfo.ColumnGoType(sourceType) != "time.Time" {
				return field, fmt.Errorf("%s is not a time, and cannot be truncated to a %s", path, unit)
			}
			if column, err = c.truncateTime(column, unit); err != nil {
				return field, err
			}
		}
		field.expression = coalesce(c.sqlDialect, column, structField.Type)
		field.group = true
	case "agg":
		function, path, hasField := strings.Cut(spec, ":")
		if function == "count" && !hasField {
			field.expression = "COUNT(*)"
			return field, nil
		}

		switch function {
		case "count", "sum", "avg", "min", "max":
		default:
			return field, fmt.Errorf("unknown aggregate function %q, want count, sum, avg, min or max", function)
		}
		if !hasField {
			return field, fmt.Errorf("agg:%s needs a field, e.g. agg:%s:Total", function, function)
		}
		if strings.Contains(path, ".") {
			return field, fmt.Errorf("agg:%s: %s: only fields of the struct can be aggregated", function, path)
		}

		column, _, err := c.aggregateColumn(builder, joins, path)
		if err != nil {
			return field, err
		}
		field.expression = strings.ToUpper(function) + "(" + column + ")"
		if function != "count" {
			field.expression = coalesce(c.sqlDialect, field.expression, structField.Type)
		}
	default:
		return field, fmt.Errorf("the %s tag must be group:<field> or agg:<function>, got %q", c.tagName, tag)
	}

	return field, nil
}

// aggregateColumn returns the qualified column of a field of the struct, or of a path through belongs-to relations,
// for which a LEFT JOIN is added, and the type of the field.
func (c *CRUD) aggregateColumn(builder *sqlbuilder.Builder, joins *aggregateJoins, path string) (string, reflect.Type, error) {
	segments := strings.Split(path, ".")
	current, currentType := builder, joins.objType
	prefix := ""
	for i, segment := range segments[:len(segments)-1] {
		relation := findRelation(current, segment)
		if relation == nil || relation.Kind != structinfo.BelongsTo {
			return "", nil, fmt.Errorf("%s: %s is not a belongs-to relation field", path, segment)
		}

		related, err := c.builder(reflect.New(relation.Type).Interface())
		if err != nil {
			return "", nil, err
		}

		next := strings.Join(segments[:i+1], ".")
		if _, ok := joins.aliases[next]; !ok {
			alias := fmt.Sprintf("g%d", len(joins.aliases)+1)
			relatedRows, err := joins.dialects[prefix].Relation(segment, alias)
			if err != nil {
				return "", nil, err
			}
			joins.aliases[next] = alias
			joins.dialects[next] = relatedRows.Dialect
			fmt.Fprintf(&joins.sql, " LEFT JOIN %s %s ON %s", relatedRows.Table, alias, relatedRows.Match)
		}

		current, currentType, prefix = related, relation.Type, next
	}

	name := segments[len(segments)-1]
	if !slices.Contains(current.ColumnFieldNames(), name) {
		return "", nil, fmt.Errorf("%s is not a stored field", path)
	}
	if slices.Contains(current.PasswordFields(), name) {
		return "", nil, fmt.Errorf("%s is a password field", path)
	}

	column, err := joins.dialects[prefix].Column(name)
	if err != nil {
		return "", nil, err
	}

	fieldType, _ := currentType.FieldByName(name)
	return column, fieldType.Type, nil
}

// truncateTime returns the expression of a time column truncated to a unit, as a time in UTC.
func (c *CRUD) truncateTime(column string, unit string) (string, error) {
	if expression, ok := c.sqlDialect.TruncateTime(column, unit); ok {
		return expression, nil
	}
	return "", fmt.Errorf("unknown unit %q, want day, month or year", unit)
}

// coalesce returns an expression with the zero value of a field instead of NULL, e.g. '0.00' for a decimal type,
// unless NULL is a value of the field: a pointer, or a Valuer type whose zero value is NULL. Times are not changed, as
// they have no zero value in SQL.
func coalesce(d dialect.Dialect, expression string, fieldType reflect.Type) string {
	goType := structinfo.ColumnGoType(fieldType)
	if fieldType.Kind() == reflect.Pointer || structinfo.IsNullableType(fieldType) || goType == "time.Time" {
		return expression
	}

	_, zero := d.ColumnType(goType)
	if structinfo.IsValuerType(fieldType) {
		value, err := structinfo.ZeroValue(fieldType)
		if err != nil {
			return expression
		}
		zero = d.Literal(value)
	}
	return "COALESCE(" + expression + "," + zero + ")"
}

// scanAggregateRow scans a row of Aggregate into result. A database that stores times as text in timeFormat, SQLite,
// returns the times of expressions as text.
func scanAggregateRow(rows *sql.Rows, result reflect.Value, fields []aggregateField, timeFormat string) error {
	destinations := make([]interface{}, len(fields))
	texts := make([]sql.NullString, len(fields))
	for i, field := range fields {
		if field.isTime && timeFormat != "" {
			destinations[i] = &texts[i]
			continue
		}
		value := result.FieldByName(field.name)
		if structinfo.IsNamedBoolType(value.Type()) {
			destinations[i] = namedBoolScanner{field: value}
			continue
		}
		destinations[i] = value.Addr().Interface()
	}

	if err := rows.Scan(destinations...); err != nil {
		return err
	}

	for i, field := range fields {
		value := result.FieldByName(field.name)
		if !field.isTime {
			continue
		}

		if timeFormat != "" {
			if err := setTimeText(value, texts[i], timeFormat); err != nil {
				return fmt.Errorf("%s: %w", field.name, err)
			}
			continue
		}
		// a Valuer has the time that it scans
		if structinfo.IsValuerType(structinfo.BaseType(value.Type())) {
			continue
		}

		if value.Kind() == reflect.Pointer {
			if !value.IsNil() {
				setTime(value, value.Elem().Interface().(time.Time).UTC())
			}
		} else {
			setTime(value, value.Interface().(time.Time).UTC())
		}
	}

	return nil
}

// setTimeText sets a field of a time to a time as text in timeFormat. A field of a Valuer type scans the time, or NULL.
func setTimeText(value reflect.Value, text sql.NullString, timeFormat string) error {
	var t interface{}
	if text.Valid {
		parsed, err := time.Parse(timeFormat, text.String)
		if err != nil {
			return err
		}
		t = parsed.UTC()
	}

	if valuerType := structinfo.BaseType(value.Type()); structinfo.IsValuerType(valuerType) {
		if t == nil && value.Kind() == reflect.Pointer {
			value.Set(reflect.Zero(value.Type()))
			return nil
		}
		scanned := reflect.New(valuerType)
		if err := scanned.Interface().(sql.Scanner).Scan(t); err != nil {
			return err
		}
		if value.Kind() == reflect.Pointer {
			value.Set(scanned)
		} else {
			value.Set(scanned.Elem())
		}
		return nil
	}

	if t == nil {
		if value.Kind() != reflect.Pointer {
			return errors.New("NULL, use a *time.Time field")
		}
		value.Set(reflect.Zero(value.Type()))
		return nil
	}
	setTime(value, t.(time.Time))
	return nil
}

func setTime(value reflect.Value, t time.Time) {
	if value.Kind() == reflect.Pointer {
		value.Set(reflect.ValueOf(&t))
		return
	}
	value.Set(reflect.ValueOf(t))
}
