package sqlfilters

import "sort"

// Filter is a condition of a query: Filters, *Filters, a group made with And or Or, or a raw condition made with Raw.
type Filter interface {
	isFilter()
}

func (Filters) isFilter() {}

// Group joins filters with AND, or with OR. It is made with And and Or.
type Group struct {
	Or      bool
	Filters []Filter
}

func (Group) isFilter() {}

// And returns a group of filters that are all true. An empty group matches every row.
func And(filters ...Filter) Group {
	return Group{Filters: filters}
}

// Or returns a group of filters of which at least one is true. An empty group matches no rows.
func Or(filters ...Filter) Group {
	return Group{Or: true, Filters: filters}
}

// RawFilter is a fragment of SQL. It is made with Raw.
type RawFilter struct {
	SQL    string
	Values []interface{}
}

func (RawFilter) isFilter() {}

// Raw returns a fragment of SQL, e.g. Raw(".Price > ? OR .Stock IN (?)", 100, []int{1, 2}). Fields are written as
// .FieldName, and each ? is replaced with a value, or with a list of values when the value is a slice or an array.
// The SQL is used as it is, so it must never be built from user input.
func Raw(sql string, values ...interface{}) RawFilter {
	return RawFilter{SQL: sql, Values: values}
}

// SubqueryValue is the value of an OpIn condition that is a subquery. It is made with Subquery.
type SubqueryValue struct {
	Obj    interface{}
	Field  string
	Filter Filter
}

// Subquery returns the value of an OpIn condition that matches the values of a field of the objects of another struct
// that match a filter, e.g. the customers that have paid orders:
//
//	Filters{"ID": {Op: OpIn, Val: Subquery(&Order{}, "CustomerID", Filters{"Status": {Op: OpEqual, Val: "paid"}})}}
func Subquery(obj interface{}, field string, filter Filter) SubqueryValue {
	return SubqueryValue{Obj: obj, Field: field, Filter: filter}
}

// Condition is a condition on a field, i.e. an entry of Filters.
type Condition struct {
	Field string
	OpVal
}

// Conditions returns the conditions on fields of a filter, in the order in which they are in the query: the filters of
// a group in their order, and the entries of Filters sorted by field name. Raw filters are not included.
func Conditions(f Filter) []Condition {
	var conditions []Condition
	walk(f, func(c Condition) {
		conditions = append(conditions, c)
	}, func(RawFilter) {})

	return conditions
}

// walk calls condFunc for each condition on a field, and rawFunc for each raw filter, in the order of the query.
func walk(f Filter, condFunc func(Condition), rawFunc func(RawFilter)) {
	switch f := f.(type) {
	case *Filters:
		if f != nil {
			walk(*f, condFunc, rawFunc)
		}
	case Filters:
		names := make([]string, 0, len(f))
		for name := range f {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			condFunc(Condition{Field: name, OpVal: f[name]})
		}
	case Group:
		for _, filter := range f.Filters {
			walk(filter, condFunc, rawFunc)
		}
	case RawFilter:
		rawFunc(f)
	}
}

// FiltersInterfaces returns the values of a filter, in the order of the placeholders in the query: one value for each
// condition, none for IS NULL and IS NOT NULL conditions, one for each element of OpIn, the values of the filter of a
// subquery, and the values of raw filters, with one for each element of a slice or an array.
func FiltersInterfaces(f Filter) []interface{} {
	var interfaces []interface{}

	walk(f, func(c Condition) {
		switch {
		case c.IsNull():
		case c.Op == OpIn:
			if subquery, ok := c.Val.(SubqueryValue); ok {
				interfaces = append(interfaces, FiltersInterfaces(subquery.Filter)...)
				break
			}
			values, _ := c.InValues()
			interfaces = append(interfaces, values...)
		default:
			interfaces = append(interfaces, c.Val)
		}
	}, func(raw RawFilter) {
		for _, value := range raw.Values {
			if values, ok := (OpVal{Val: value}).InValues(); ok {
				interfaces = append(interfaces, values...)
				continue
			}
			interfaces = append(interfaces, value)
		}
	})

	return interfaces
}
