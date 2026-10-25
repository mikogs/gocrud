package gocrud

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type PagedPost struct {
	ID          uint64
	Title       string
	Score       int64
	PublishedAt time.Time
	Price       Money `crud:"type:numeric(12,2)"`
	Ref         uuid.UUID
	Note        *string
	Tags        []string `crud:"json"`
}

func testCursor(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[PagedPost](ctx, crud)
	if err := CreateTable[PagedPost](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	start := time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC)
	posts := make([]*PagedPost, 25)
	for i := range posts {
		// few scores, times and prices, so that many posts have the same values
		// titles that are not in the order of the IDs, and some of them the same
		posts[i] = &PagedPost{Title: fmt.Sprintf("post %d", i*7%10), Score: int64(i % 4), PublishedAt: start.Add(time.Duration(i%3) * time.Hour),
			Price: Money{cents: int64(i % 5 * 150)}, Ref: uuid.New()}
	}
	if err := InsertMultiple(ctx, crud, posts, InsertMultipleOptions{}); err != nil {
		t.Fatalf("InsertMultiple failed: %s", err.Error())
	}

	ids := func(posts []*PagedPost) []uint64 {
		list := make([]uint64, len(posts))
		for i, post := range posts {
			list[i] = post.ID
		}
		return list
	}
	// pages returns the IDs of the posts of all the pages of a Get with order
	pages := func(t *testing.T, order []string, filters sqlfilters.Filter) []uint64 {
		t.Helper()
		var all []uint64
		after := ""
		for page := 0; ; page++ {
			if page > len(posts) {
				t.Fatalf("Get did not return the last page")
			}
			got, err := Get[PagedPost](ctx, crud, GetOptions{Order: order, Limit: 7, After: after, Filters: filters})
			if err != nil {
				t.Fatalf("Get of the page %d failed: %s", page, err.Error())
			}
			all = append(all, ids(got)...)
			if len(got) < 7 {
				return all
			}
			if after, err = crud.NextCursor(got[len(got)-1], order); err != nil {
				t.Fatalf("NextCursor failed: %s", err.Error())
			}
		}
	}

	for name, test := range map[string]struct {
		order     []string
		fullOrder []string
		filters   sqlfilters.Filter
	}{
		"ID":                {nil, []string{"ID", "asc"}, nil},
		"one field":         {[]string{"Score", "desc"}, []string{"Score", "desc", "ID", "desc"}, nil},
		"upper case":        {[]string{"Score", "DESC", "Title", "ASC"}, []string{"Score", "DESC", "Title", "ASC", "ID", "ASC"}, nil},
		"three fields":      {[]string{"Title", "desc", "PublishedAt", "asc", "Score", "desc"}, []string{"Title", "desc", "PublishedAt", "asc", "Score", "desc", "ID", "desc"}, nil},
		"mixed directions":  {[]string{"PublishedAt", "asc", "Title", "desc"}, []string{"PublishedAt", "asc", "Title", "desc", "ID", "desc"}, nil},
		"ID in the order":   {[]string{"ID", "desc", "Score", "asc"}, []string{"ID", "desc"}, nil},
		"Valuer":            {[]string{"Price", "asc", "Score", "asc"}, []string{"Price", "asc", "Score", "asc", "ID", "asc"}, nil},
		"UUID":              {[]string{"Ref", "asc"}, []string{"Ref", "asc", "ID", "asc"}, nil},
		"with filters":      {[]string{"Score", "asc"}, []string{"Score", "asc", "ID", "asc"}, sqlfilters.Filters{"Score": {Op: sqlfilters.OpGreaterOrEqual, Val: 2}}},
		"with an OR filter": {[]string{"Score", "asc"}, []string{"Score", "asc", "ID", "asc"}, sqlfilters.Or(sqlfilters.Filters{"Score": {Op: sqlfilters.OpEqual, Val: 0}}, sqlfilters.Filters{"Score": {Op: sqlfilters.OpEqual, Val: 3}})},
	} {
		t.Run(name, func(t *testing.T) {
			all, err := Get[PagedPost](ctx, crud, GetOptions{Order: test.fullOrder, Filters: test.filters})
			if err != nil {
				t.Fatalf("Get failed: %s", err.Error())
			}
			if got := pages(t, test.order, test.filters); !reflect.DeepEqual(got, ids(all)) {
				t.Fatalf("the pages returned %v, want %v", got, ids(all))
			}
		})
	}

	// the values of the cursor are not strings, also when the filters are
	fromStrings := sqlfilters.Filters{"Score": {Op: sqlfilters.OpGreaterOrEqual, Val: "2"}}
	first, err := Get[PagedPost](ctx, crud, GetOptions{Order: []string{"Score", "asc"}, Limit: 3, Filters: fromStrings, ConvertFiltersFromString: true})
	if err != nil || len(first) != 3 {
		t.Fatalf("Get with filters from strings returned %d posts, %v", len(first), err)
	}
	cursor, err := crud.NextCursor(first[2], []string{"Score", "asc"})
	if err != nil {
		t.Fatalf("NextCursor failed: %s", err.Error())
	}
	next, err := Get[PagedPost](ctx, crud, GetOptions{Order: []string{"Score", "asc"}, Limit: 3, After: cursor, Filters: fromStrings, ConvertFiltersFromString: true})
	if err != nil || len(next) != 3 || next[0].Score < 2 || next[0].ID == first[2].ID {
		t.Fatalf("Get after a cursor with filters from strings returned %v, %v", ids(next), err)
	}

	// the objects of a page without an order are ordered by ID
	var queries []string
	hooked := New(crud.db, Options{Dialect: crud.dialect, QueryHook: QueryHookFunc(func(_ context.Context, event *QueryEvent) {
		queries = append(queries, event.Query)
	})})
	page, err := Get[PagedPost](ctx, hooked, GetOptions{Limit: 30})
	if err != nil || !slices.IsSorted(ids(page)) || len(page) != len(posts) || len(queries) != 1 || !strings.Contains(queries[0], `ORDER BY "id" ASC`) {
		t.Fatalf("Get of a page without an order returned %v, %v, with %v, want the posts by ID", ids(page), err, queries)
	}

	cursor, err = crud.NextCursor(posts[3], []string{"Score", "desc"})
	if err != nil {
		t.Fatalf("NextCursor failed: %s", err.Error())
	}
	for name, options := range map[string]GetOptions{
		"another order":  {Order: []string{"Score", "asc"}, After: cursor},
		"not a cursor":   {Order: []string{"Score", "desc"}, After: "not a cursor!"},
		"not JSON":       {Order: []string{"Score", "desc"}, After: "bm90IGpzb24"},
		"a wrong value":  {Order: []string{"Score", "desc"}, After: "eyJvIjpbIlNjb3JlIiwiZGVzYyIsIklEIiwiZGVzYyJdLCJ2IjpbIngiLDFdfQ"},
		"with an offset": {Order: []string{"Score", "desc"}, After: cursor, Offset: 7},
		"a nullable":     {Order: []string{"Note", "asc"}, After: cursor},
		"a JSON field":   {Order: []string{"Tags", "asc"}, After: cursor},
		"too few values": {Order: []string{"Score", "desc"}, After: "eyJvIjpbIlNjb3JlIiwiZGVzYyIsIklEIiwiZGVzYyJdLCJ2IjpbMV19"},
	} {
		if _, err := Get[PagedPost](ctx, crud, options); !errors.Is(err, ErrCursor) {
			t.Fatalf("Get with %s returned %v, want ErrCursor", name, err)
		}
	}
	for _, order := range [][]string{{"Note", "asc"}, {"Tags", "asc"}} {
		if _, err := crud.NextCursor(posts[0], order); !errors.Is(err, ErrCursor) {
			t.Fatalf("NextCursor with the order %v returned %v, want ErrCursor", order, err)
		}
	}
	if _, err := crud.NextCursor(&RelOrder{ID: 1}, []string{"Customer.Name", "asc"}); !errors.Is(err, ErrCursor) {
		t.Fatalf("NextCursor with a field of a related object returned %v, want ErrCursor", err)
	}
	if _, err := crud.NextCursor(posts[0], []string{"Password", "asc"}); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("NextCursor with a field that does not exist returned %v, want ErrInvalidOrder", err)
	}
}

func TestCursor(t *testing.T)        { testCursor(t, testCRUD) }
func TestSQLite_Cursor(t *testing.T) { testCursor(t, testCRUDSQLite) }

func TestPageOrder(t *testing.T) {
	for _, test := range []struct {
		order []string
		page  bool
		want  []string
	}{
		{nil, false, nil},
		{nil, true, []string{"ID", "asc"}},
		{[]string{"Score", "desc"}, false, []string{"Score", "desc", "ID", "desc"}},
		{[]string{"ID", "desc", "Score", "asc"}, true, []string{"ID", "desc", "Score", "asc"}},
	} {
		if got := pageOrder(test.order, test.page); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("pageOrder(%v, %t) returned %v, want %v", test.order, test.page, got, test.want)
		}
	}
}

func TestCursorFilter(t *testing.T) {
	builder, err := testCRUDSQLite.builder(&PagedPost{})
	if err != nil {
		t.Fatalf("builder failed: %s", err.Error())
	}
	post := &PagedPost{ID: 5, Score: 3}
	order := []string{"Score", "desc", "ID", "desc"}
	after, err := testCRUDSQLite.NextCursor(post, order[:2])
	if err != nil {
		t.Fatalf("NextCursor failed: %s", err.Error())
	}
	got, err := cursorFilter(builder, post, order, after)
	if err != nil {
		t.Fatalf("cursorFilter failed: %s", err.Error())
	}
	want := sqlfilters.And(
		sqlfilters.Filters{"Score": {Op: sqlfilters.OpLowerOrEqual, Val: int64(3)}},
		sqlfilters.Or(
			sqlfilters.Filters{"Score": {Op: sqlfilters.OpLower, Val: int64(3)}},
			sqlfilters.Filters{"Score": {Op: sqlfilters.OpEqual, Val: int64(3)}, "ID": {Op: sqlfilters.OpLower, Val: uint64(5)}},
		),
	)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cursorFilter returned %#v, want %#v", got, want)
	}
}
