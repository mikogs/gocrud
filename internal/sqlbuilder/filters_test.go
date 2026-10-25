package sqlbuilder

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/mikogs/gocrud/internal/sqlbuilder/where"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

var regexpPostgresPlaceholder = regexp.MustCompile(`\$[0-9]+`)

func TestSQLFilterGroups(t *testing.T) {
	tests := map[string]struct {
		filter   sqlfilters.Filter
		postgres string
	}{
		"or of maps": {
			sqlfilters.Or(
				sqlfilters.Filters{"FirstName": {Op: sqlfilters.OpLike, Val: "Jo%"}},
				sqlfilters.Filters{"LastName": {Op: sqlfilters.OpLike, Val: "Jo%"}, "Age": {Op: sqlfilters.OpGreater, Val: 18}},
			),
			`"first_name" LIKE $1 OR ("age">$2 AND "last_name" LIKE $3)`,
		},
		"several conditions on one field": {
			sqlfilters.And(
				sqlfilters.Filters{"Price": {Op: sqlfilters.OpGreaterOrEqual, Val: 10}},
				sqlfilters.Filters{"Price": {Op: sqlfilters.OpLower, Val: 100}},
			),
			`"price">=$1 AND "price"<$2`,
		},
		"nested groups": {
			sqlfilters.And(
				sqlfilters.Filters{"Flags": {Op: sqlfilters.OpBit, Val: 4}},
				sqlfilters.Or(
					sqlfilters.Filters{"Age": {Op: sqlfilters.OpIn, Val: []int{18, 21}}},
					sqlfilters.And(sqlfilters.Filters{"Price": {Op: sqlfilters.OpEqual, Val: 0}}, sqlfilters.Raw(".Key != ?", "")),
				),
			),
			`"flags"&$1>0 AND ("age" IN ($2,$3) OR ("price"=$4 AND ("key" != $5)))`,
		},
		"raw in or": {
			sqlfilters.Or(sqlfilters.Raw(".ID IN (?)", []uint64{1, 2}), sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 1}}),
			`("id" IN ($1,$2)) OR "age"=$3`,
		},
		"group of one": {
			sqlfilters.Or(sqlfilters.And(sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 1}})),
			`"age"=$1`,
		},
		"empty groups": {
			sqlfilters.And(sqlfilters.And(), sqlfilters.Filters{}, sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 1}}),
			`"age"=$1`,
		},
		"empty or": {
			sqlfilters.And(sqlfilters.Or(), sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 1}}),
			`1=0 AND "age"=$1`,
		},
		"empty and in or": {
			sqlfilters.Or(sqlfilters.And(), sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 1}}),
			`1=1 OR "age"=$1`,
		},
		"fields in raw": {
			sqlfilters.Raw(".PostCode = ? OR .PostCode2 = ? OR t1.age > 1.5", "a", "b"),
			`"post_code" = $1 OR "post_code2" = $2 OR t1.age > 1.5`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			wants := map[string]string{
				DialectPostgres: test.postgres,
				DialectSQLite:   regexpPostgresPlaceholder.ReplaceAllString(test.postgres, "?"),
			}
			for dialect, want := range wants {
				got, err := mustNew(t, testStructObj, Options{Dialect: dialect}).SelectCount(test.filter)
				want = `SELECT COUNT(*) AS cnt FROM "test_struct" WHERE ` + want + ";"
				if err != nil || got != want {
					t.Fatalf("%s: SelectCount returned %v\nwant %s\ngot  %s", dialect, err, want, got)
				}
			}

			if got, want := len(sqlfilters.FiltersInterfaces(test.filter)), strings.Count(test.postgres, "$"); got != want {
				t.Fatalf("FiltersInterfaces returned %d values for %d placeholders", got, want)
			}
		})
	}
}

func TestSQLFilterWithoutConditions(t *testing.T) {
	var nilFilters *sqlfilters.Filters
	for name, f := range map[string]sqlfilters.Filter{
		"nil":          nil,
		"nil *Filters": nilFilters,
		"empty And":    sqlfilters.And(sqlfilters.Filters{}),
		"empty raw":    sqlfilters.Raw(""),
	} {
		got, err := mustNew(t, testStructObj, Options{}).SelectCount(f)
		if want := `SELECT COUNT(*) AS cnt FROM "test_struct";`; err != nil || got != want {
			t.Fatalf("%s: SelectCount returned %q, %v, want %q", name, got, err, want)
		}
	}
}

func TestSQLUpdateWithFilterGroups(t *testing.T) {
	f := sqlfilters.Or(sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: 1}}, sqlfilters.Raw(".Price IN (?)", []int{1, 2}))

	got, err := mustNew(t, testStructObj, Options{}).Update(map[string]any{"FirstName": "a", "LastName": "b"}, f)
	want := `UPDATE "test_struct" SET "first_name"=$1,"last_name"=$2 WHERE "age"=$3 OR ("price" IN ($4,$5));`
	if err != nil || got != want {
		t.Fatalf("Update returned %v\nwant %s\ngot  %s", err, want, got)
	}
}

func TestSQLInvalidFilters(t *testing.T) {
	tests := map[string]struct {
		filter  sqlfilters.Filter
		dialect string
		err     error
	}{
		"too few values for raw":  {sqlfilters.Raw(".Age > ? AND .Price < ?", 1), DialectPostgres, where.ErrRawFilter},
		"too many values for raw": {sqlfilters.Raw(".Age > ?", 1, 2), DialectSQLite, where.ErrRawFilter},
		"unknown field in raw":    {sqlfilters.Raw(".Unknown > ?", 1), DialectPostgres, nil},
		"unknown field in group":  {sqlfilters.Or(sqlfilters.Filters{"Unknown": {Op: sqlfilters.OpEqual, Val: 1}}), DialectPostgres, nil},
		"in that is not a list":   {sqlfilters.And(sqlfilters.Filters{"Age": {Op: sqlfilters.OpIn, Val: 1}}), DialectSQLite, where.ErrInFilter},
		"match in sqlite":         {sqlfilters.Filters{"FirstName": {Op: sqlfilters.OpMatch, Val: "^J"}}, DialectSQLite, where.ErrMatchFilter},
	}

	for name, test := range tests {
		_, err := mustNew(t, testStructObj, Options{Dialect: test.dialect}).SelectCount(test.filter)
		if err == nil || (test.err != nil && !errors.Is(err, test.err)) {
			t.Fatalf("%s: SelectCount returned err %v, want %v", name, err, test.err)
		}
	}
}
