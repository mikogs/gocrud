package bench

import (
	"database/sql/driver"
	"reflect"
	"strings"
	"testing"

	"github.com/mikogs/gocrud"
)

type query struct {
	sql  string
	args []any
}

// TestSameQueries checks that the database/sql code of each benchmark sends the same queries, with the same
// arguments, as gocrud, so that the benchmarks compare the same work.
func TestSameQueries(t *testing.T) {
	var queries []query
	recordQuery = func(sql string, args []driver.NamedValue) {
		values := make([]any, len(args))
		for i, arg := range args {
			values[i] = arg.Value
		}
		queries = append(queries, query{strings.TrimSuffix(sql, ";"), values})
	}
	defer func() { recordQuery = nil }()

	for _, dialect := range []string{gocrud.DialectPostgres, gocrud.DialectSQLite} {
		for _, o := range operations {
			op := o.operation(newEnv(openFake(o.fakeRows), dialect))

			queries = nil
			if err := op.gocrud(); err != nil {
				t.Fatalf("%s %s: gocrud failed: %s", dialect, o.name, err.Error())
			}
			gocrudQueries := queries

			queries = nil
			if err := op.sql(); err != nil {
				t.Fatalf("%s %s: database/sql failed: %s", dialect, o.name, err.Error())
			}

			if !reflect.DeepEqual(gocrudQueries, queries) {
				t.Errorf("%s %s: database/sql sent\n%v\nwant the queries of gocrud\n%v", dialect, o.name, queries, gocrudQueries)
			}
		}
	}
}
