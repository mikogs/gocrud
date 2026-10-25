package sqlbuilder

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mikogs/gocrud/internal/sqlbuilder/where"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type PathCustomer struct {
	ID      uint64
	Name    string
	Country string
	Orders  []*PathOrder `crud:"rel:CustomerID"`
}

type PathOrder struct {
	ID         uint64
	CustomerID uint64
	Status     string
	Total      int64
	Customer   *PathCustomer `crud:"rel:CustomerID"`
	Items      []PathItem    `crud:"rel:OrderID"`
}

type PathItem struct {
	ID        uint64
	OrderID   uint64
	ProductID uint64
	Quantity  int64
	Product   *PathProduct `crud:"rel:ProductID"`
	Order     *PathOrder   `crud:"rel:OrderID"`
}

type PathProduct struct {
	ID       uint64
	Category string
}

// newPathBuilder returns a builder of obj that gets the builders of related structs as gocrud does.
func newPathBuilder(t *testing.T, obj any, dialect string) *Builder {
	t.Helper()

	builders := map[reflect.Type]*Builder{}
	var options Options
	options = Options{Dialect: dialect, RelatedBuilder: func(typ reflect.Type) (*Builder, error) {
		if b, ok := builders[typ]; ok {
			return b, nil
		}
		b, err := New(reflect.New(typ).Interface(), options)
		builders[typ] = b
		return b, err
	}}

	b, err := New(obj, options)
	if err != nil {
		t.Fatalf("New failed: %s", err.Error())
	}
	return b
}

func TestSQLRelationPaths(t *testing.T) {
	tests := map[string]struct {
		obj    any
		filter sqlfilters.Filter
		want   string
	}{
		"belongs to": {&PathOrder{}, sqlfilters.Filters{
			"Customer.Country": {Op: sqlfilters.OpEqual, Val: "PL"},
		}, `EXISTS (SELECT 1 FROM "path_customer" r1 WHERE r1."id" = "path_order"."customer_id" AND r1."country"=$1)`},
		"has many, one related row": {&PathCustomer{}, sqlfilters.Filters{
			"Orders.Status": {Op: sqlfilters.OpEqual, Val: "paid"},
			"Orders.Total":  {Op: sqlfilters.OpGreater, Val: 100},
		}, `EXISTS (SELECT 1 FROM "path_order" r1 WHERE r1."customer_id" = "path_customer"."id" AND r1."status"=$1 AND r1."total">$2)`},
		"has many, any related rows": {&PathCustomer{}, sqlfilters.And(
			sqlfilters.Filters{"Orders.Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
			sqlfilters.Filters{"Orders.Total": {Op: sqlfilters.OpGreater, Val: 100}},
		), `EXISTS (SELECT 1 FROM "path_order" r1 WHERE r1."customer_id" = "path_customer"."id" AND r1."status"=$1) AND ` +
			`EXISTS (SELECT 1 FROM "path_order" r2 WHERE r2."customer_id" = "path_customer"."id" AND r2."total">$2)`},
		"nested": {&PathOrder{}, sqlfilters.Filters{
			"Items.Product.Category": {Op: sqlfilters.OpIn, Val: []string{"books", "games"}},
			"Items.Quantity":         {Op: sqlfilters.OpGreaterOrEqual, Val: 2},
		}, `EXISTS (SELECT 1 FROM "path_item" r1 WHERE r1."order_id" = "path_order"."id" AND ` +
			`EXISTS (SELECT 1 FROM "path_product" r2 WHERE r2."id" = r1."product_id" AND r2."category" IN ($1,$2)) AND r1."quantity">=$3)`},
		"in or": {&PathOrder{}, sqlfilters.Or(
			sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "new"}},
			sqlfilters.Filters{"Customer.Country": {Op: sqlfilters.OpEqual, Val: nil}},
		), `"status"=$1 OR EXISTS (SELECT 1 FROM "path_customer" r1 WHERE r1."id" = "path_order"."customer_id" AND r1."country" IS NULL)`},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := newPathBuilder(t, test.obj, DialectPostgres).SelectCount(test.filter)
			if err != nil || !strings.HasSuffix(got, " WHERE "+test.want+";") {
				t.Fatalf("SelectCount returned %v\n%s\nwant the conditions\n%s", err, got, test.want)
			}

			sqlite, err := newPathBuilder(t, test.obj, DialectSQLite).SelectCount(test.filter)
			if want := regexpPostgresPlaceholder.ReplaceAllString(test.want, "?"); err != nil || !strings.HasSuffix(sqlite, " WHERE "+want+";") {
				t.Fatalf("SelectCount for SQLite returned %v\n%s\nwant the conditions\n%s", err, sqlite, want)
			}

			if got, want := len(sqlfilters.FiltersInterfaces(test.filter)), strings.Count(test.want, "$"); got != want {
				t.Fatalf("FiltersInterfaces returned %d values for %d placeholders", got, want)
			}
		})
	}
}

func TestSQLRelationPathsOrderOfValues(t *testing.T) {
	f := sqlfilters.Filters{
		"CustomerID":    {Op: sqlfilters.OpNotEqual, Val: 0},
		"Customer.Name": {Op: sqlfilters.OpLike, Val: "A%"},
		"Status":        {Op: sqlfilters.OpEqual, Val: "paid"},
	}

	got, err := newPathBuilder(t, &PathOrder{}, DialectSQLite).SelectCount(f)
	want := `WHERE EXISTS (SELECT 1 FROM "path_customer" r1 WHERE r1."id" = "path_order"."customer_id" AND r1."name" LIKE ?) AND "customer_id"!=? AND "status"=?;`
	if err != nil || !strings.HasSuffix(got, want) {
		t.Fatalf("SelectCount returned %v\n%s\nwant the conditions in the order of the values\n%s", err, got, want)
	}
	if values := sqlfilters.FiltersInterfaces(f); !reflect.DeepEqual(values, []any{"A%", 0, "paid"}) {
		t.Fatalf("FiltersInterfaces returned %v, want the values in the order of the conditions", values)
	}
}

func TestSQLRelationPathsInUpdateAndDelete(t *testing.T) {
	f := sqlfilters.Filters{"Customer.Country": {Op: sqlfilters.OpEqual, Val: "PL"}}
	b := newPathBuilder(t, &PathOrder{}, DialectPostgres)

	update, err := b.Update(map[string]any{"Status": "cancelled"}, f)
	if want := `UPDATE "path_order" SET "status"=$1 WHERE EXISTS (SELECT 1 FROM "path_customer" r1 WHERE r1."id" = "path_order"."customer_id" AND r1."country"=$2);`; err != nil || update != want {
		t.Fatalf("Update returned %v\n%s\nwant\n%s", err, update, want)
	}

	deleteQuery, err := b.DeleteReturningID(f)
	if want := `DELETE FROM "path_order" WHERE EXISTS (SELECT 1 FROM "path_customer" r1 WHERE r1."id" = "path_order"."customer_id" AND r1."country"=$1) RETURNING "id";`; err != nil || deleteQuery != want {
		t.Fatalf("DeleteReturningID returned %v\n%s\nwant\n%s", err, deleteQuery, want)
	}
}

func TestSQLInvalidRelationPaths(t *testing.T) {
	tests := map[string]sqlfilters.Filter{
		"not a relation":            sqlfilters.Filters{"Status.Name": {Op: sqlfilters.OpEqual, Val: "x"}},
		"unknown field of relation": sqlfilters.Filters{"Customer.Unknown": {Op: sqlfilters.OpEqual, Val: "x"}},
		"unknown nested relation":   sqlfilters.Filters{"Items.Supplier.Name": {Op: sqlfilters.OpEqual, Val: "x"}},
	}
	for name, f := range tests {
		if _, err := newPathBuilder(t, &PathOrder{}, DialectPostgres).SelectCount(f); err == nil {
			t.Fatalf("SelectCount with %s did not return an err", name)
		}
	}

	_, err := newPathBuilder(t, &PathOrder{}, DialectPostgres).SelectCount(sqlfilters.Filters{"Status.Name": {Op: sqlfilters.OpEqual, Val: "x"}})
	if !errors.Is(err, where.ErrRelationPath) || !strings.Contains(err.Error(), "Status is not a relation field of PathOrder") {
		t.Fatalf("SelectCount returned %v, want %v naming the field", err, where.ErrRelationPath)
	}

	withoutRelated := mustNew(t, &PathOrder{}, Options{})
	if _, err := withoutRelated.SelectCount(sqlfilters.Filters{"Customer.Name": {Op: sqlfilters.OpEqual, Val: "x"}}); !errors.Is(err, where.ErrRelationPath) {
		t.Fatalf("SelectCount without RelatedBuilder returned %v, want %v", err, where.ErrRelationPath)
	}
}

type PathOrderView struct {
	ID         uint64
	CustomerID uint64
	Customer   *PathCustomer `crud:"rel:CustomerID"`
}

func (o *PathOrderView) BuildSelectPrefixQuery(tableNamePrefix string) (string, error) {
	return `SELECT "id","customer_id" FROM "` + tableNamePrefix + `visible_order"`, nil
}

func TestSQLRelationPathsWithOwnSelectQuery(t *testing.T) {
	_, err := newPathBuilder(t, &PathOrderView{}, DialectPostgres).Select(nil, 0, 0, sqlfilters.Filters{"Customer.Name": {Op: sqlfilters.OpEqual, Val: "x"}})
	if !errors.Is(err, where.ErrRelationPath) || !strings.Contains(err.Error(), "the struct has its own select query") {
		t.Fatalf("Select of a struct with its own select query returned %v, want %v", err, where.ErrRelationPath)
	}
}

func TestSQLRelationPathsInOrder(t *testing.T) {
	tests := map[string]struct {
		obj   any
		order []string
		want  string
	}{
		"belongs to": {&PathOrder{}, []string{"Customer.Name", "asc", "ID", "desc"},
			`ORDER BY (SELECT r1."name" FROM "path_customer" r1 WHERE r1."id" = "path_order"."customer_id") ASC NULLS LAST,"id" DESC`},
		"nested": {&PathItem{}, []string{"Order.Customer.Country", "desc"},
			`ORDER BY (SELECT (SELECT r2."country" FROM "path_customer" r2 WHERE r2."id" = r1."customer_id") ` +
				`FROM "path_order" r1 WHERE r1."id" = "path_item"."order_id") DESC NULLS LAST`},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			for _, dialect := range []string{DialectPostgres, DialectSQLite} {
				got, err := newPathBuilder(t, test.obj, dialect).Select(test.order, 0, 0, nil)
				if err != nil || !strings.HasSuffix(got, " "+test.want+";") {
					t.Fatalf("%s: Select returned %v\n%s\nwant\n%s", dialect, err, got, test.want)
				}
			}
		})
	}

	invalid := map[string]string{
		"has many":       "Items.Quantity",
		"not a relation": "Status.Name",
	}
	for name, field := range invalid {
		_, err := newPathBuilder(t, &PathOrder{}, DialectPostgres).Select([]string{field, "asc"}, 0, 0, nil)
		if !errors.Is(err, where.ErrRelationPath) {
			t.Fatalf("Select ordered by %s returned %v, want %v", name, err, where.ErrRelationPath)
		}
	}

	_, err := newPathBuilder(t, &PathOrderView{}, DialectPostgres).Select([]string{"Customer.Name", "asc"}, 0, 0, nil)
	if !errors.Is(err, where.ErrRelationPath) {
		t.Fatalf("Select of a struct with its own select query returned %v, want %v", err, where.ErrRelationPath)
	}
}

func TestSQLSubqueries(t *testing.T) {
	paid := sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}}
	tests := map[string]struct {
		obj    any
		filter sqlfilters.Filter
		want   string
	}{
		"in the order of the values": {&PathCustomer{}, sqlfilters.Filters{
			"Country": {Op: sqlfilters.OpEqual, Val: "PL"},
			"ID":      {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&PathOrder{}, "CustomerID", paid)},
			"Name":    {Op: sqlfilters.OpLike, Val: "A%"},
		}, `"country"=$1 AND "id" IN (SELECT s1."customer_id" FROM "path_order" s1 WHERE s1."status"=$2) AND "name" LIKE $3`},
		"without a filter": {&PathCustomer{}, sqlfilters.Filters{
			"ID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(PathOrder{}, "CustomerID", nil)},
		}, `"id" IN (SELECT s1."customer_id" FROM "path_order" s1)`},
		"with paths and a subquery": {&PathCustomer{}, sqlfilters.Filters{
			"ID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&PathOrder{}, "CustomerID", sqlfilters.Filters{
				"Items.Product.Category": {Op: sqlfilters.OpEqual, Val: "books"},
				"Status":                 {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&PathOrder{}, "Status", sqlfilters.Filters{"Total": {Op: sqlfilters.OpGreater, Val: 100}})},
			})},
		}, `"id" IN (SELECT s1."customer_id" FROM "path_order" s1 WHERE ` +
			`EXISTS (SELECT 1 FROM "path_item" r1 WHERE r1."order_id" = s1."id" AND ` +
			`EXISTS (SELECT 1 FROM "path_product" r2 WHERE r2."id" = r1."product_id" AND r2."category"=$1)) AND ` +
			`s1."status" IN (SELECT s2."status" FROM "path_order" s2 WHERE s2."total">$2))`},
		"in a path": {&PathOrder{}, sqlfilters.Filters{
			"Customer.ID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&PathOrder{}, "CustomerID", paid)},
		}, `EXISTS (SELECT 1 FROM "path_customer" r1 WHERE r1."id" = "path_order"."customer_id" AND ` +
			`r1."id" IN (SELECT s1."customer_id" FROM "path_order" s1 WHERE s1."status"=$1))`},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := newPathBuilder(t, test.obj, DialectPostgres).SelectCount(test.filter)
			if err != nil || !strings.HasSuffix(got, " WHERE "+test.want+";") {
				t.Fatalf("SelectCount returned %v\n%s\nwant the conditions\n%s", err, got, test.want)
			}

			sqlite, err := newPathBuilder(t, test.obj, DialectSQLite).SelectCount(test.filter)
			if want := regexpPostgresPlaceholder.ReplaceAllString(test.want, "?"); err != nil || !strings.HasSuffix(sqlite, " WHERE "+want+";") {
				t.Fatalf("SelectCount for SQLite returned %v\n%s\nwant the conditions\n%s", err, sqlite, want)
			}

			if got, want := len(sqlfilters.FiltersInterfaces(test.filter)), strings.Count(test.want, "$"); got != want {
				t.Fatalf("FiltersInterfaces returned %d values for %d placeholders", got, want)
			}
		})
	}

	values := sqlfilters.FiltersInterfaces(tests["in the order of the values"].filter)
	if !reflect.DeepEqual(values, []any{"PL", "paid", "A%"}) {
		t.Fatalf("FiltersInterfaces returned %v, want the values of the subquery between the values around it", values)
	}
}

func TestSQLInvalidSubqueries(t *testing.T) {
	tests := map[string]struct {
		val  sqlfilters.OpVal
		want string
	}{
		"not OpIn":       {sqlfilters.OpVal{Op: sqlfilters.OpEqual, Val: sqlfilters.Subquery(&PathOrder{}, "CustomerID", nil)}, "can only be the value of OpIn"},
		"not a struct":   {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(42, "CustomerID", nil)}, "int is not a struct"},
		"not stored":     {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&PathOrder{}, "Customer", nil)}, "Customer is not a stored field of PathOrder"},
		"invalid filter": {sqlfilters.OpVal{Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&PathOrder{}, "CustomerID", sqlfilters.Filters{"Unknown": {Op: sqlfilters.OpEqual, Val: 1}})}, "filter"},
	}
	for name, test := range tests {
		_, err := newPathBuilder(t, &PathCustomer{}, DialectPostgres).SelectCount(sqlfilters.Filters{"ID": test.val})
		if err == nil || !strings.Contains(err.Error(), test.want) || (name != "invalid filter" && !errors.Is(err, where.ErrSubquery)) {
			t.Fatalf("SelectCount with a subquery that is %s returned %v, want %v with %q", name, err, where.ErrSubquery, test.want)
		}
	}

	withoutRelated := mustNew(t, &PathCustomer{}, Options{})
	_, err := withoutRelated.SelectCount(sqlfilters.Filters{"ID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&PathOrder{}, "CustomerID", nil)}})
	if !errors.Is(err, where.ErrSubquery) {
		t.Fatalf("SelectCount without RelatedBuilder returned %v, want %v", err, where.ErrSubquery)
	}
}

type PathAccount struct {
	ID       uint64
	Name     string
	Password string `crud:"pass"`
}

type PathSession struct {
	ID        uint64
	AccountID uint64
	Account   *PathAccount `crud:"rel:AccountID"`
}

func TestSQLPasswordFields(t *testing.T) {
	for _, dialect := range []string{DialectPostgres, DialectSQLite} {
		tests := map[string]struct {
			obj    any
			order  []string
			filter sqlfilters.Filter
		}{
			"a condition":           {&PathAccount{}, nil, sqlfilters.Filters{"Password": {Op: sqlfilters.OpLike, Val: "$2a$%"}}},
			"a raw filter":          {&PathAccount{}, nil, sqlfilters.Raw(".Password LIKE ?", "$2a$%")},
			"a path":                {&PathSession{}, nil, sqlfilters.Filters{"Account.Password": {Op: sqlfilters.OpEqual, Val: "x"}}},
			"a subquery":            {&PathSession{}, nil, sqlfilters.Filters{"AccountID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&PathAccount{}, "Password", nil)}}},
			"an order":              {&PathAccount{}, []string{"Password", "asc"}, nil},
			"an order through path": {&PathSession{}, []string{"Account.Password", "asc"}, nil},
		}
		for name, test := range tests {
			_, err := newPathBuilder(t, test.obj, dialect).Select(test.order, 0, 0, test.filter)
			if !errors.Is(err, where.ErrPasswordField) {
				t.Fatalf("%s: Select with a password field in %s returned %v, want %v", dialect, name, err, where.ErrPasswordField)
			}
		}
	}

	raw := sqlfilters.Raw(`"password" = ''`)
	if _, err := newPathBuilder(t, &PathAccount{}, DialectPostgres).Select(nil, 0, 0, raw); err != nil {
		t.Fatalf("Select with the column of a password in a raw filter returned %v", err)
	}
}
