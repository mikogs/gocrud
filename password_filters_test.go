package gocrud

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type PFUser struct {
	ID       uint64
	Name     string
	Password string `crud:"pass"`
}

type PFPost struct {
	ID       uint64
	AuthorID uint64
	Title    string
	Author   *PFUser `crud:"rel:AuthorID"`
}

func createPasswordFilterTables(t *testing.T, crud *CRUD) {
	t.Helper()
	ctx := context.Background()

	for _, obj := range []interface{}{&PFPost{}, &PFUser{}} {
		_ = crud.DropTable(ctx, obj)
	}
	for _, obj := range []interface{}{&PFUser{}, &PFPost{}} {
		if err := crud.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed: %s", err.Error())
		}
	}
	for _, obj := range []interface{}{&PFUser{Name: "alice", Password: "secret"}, &PFPost{AuthorID: 1, Title: "first"}} {
		if err := crud.Save(ctx, obj, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}
}

func testPasswordFilters(t *testing.T, crud *CRUD) {
	createPasswordFilterTables(t, crud)
	ctx := context.Background()
	hashPrefix := sqlfilters.Filters{"Password": {Op: sqlfilters.OpLike, Val: "$2a$%"}}

	_, err := Get[PFUser](ctx, crud, GetOptions{Filters: hashPrefix})
	fatalIfNotViolations(t, err, map[string]string{"Password": ViolationPassword})

	_, err = Get[PFUser](ctx, crud, GetOptions{ConvertFiltersFromString: true, Filters: sqlfilters.Or(
		sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "alice"}},
		sqlfilters.Filters{"Password": {Op: sqlfilters.OpMatch, Val: "^\\$2a"}},
	)})
	fatalIfNotViolations(t, err, map[string]string{"Password": ViolationPassword})

	_, err = Get[PFPost](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"Author.Password": {Op: sqlfilters.OpLike, Val: "$2a$%"}}})
	fatalIfNotViolations(t, err, map[string]string{"Author.Password": ViolationPassword})

	_, err = GetCount[PFUser](ctx, crud, GetCountOptions{Filters: hashPrefix})
	fatalIfNotViolations(t, err, map[string]string{"Password": ViolationPassword})

	err = UpdateMultiple[PFUser](ctx, crud, map[string]interface{}{"Name": "bob"}, UpdateMultipleOptions{Filters: hashPrefix})
	fatalIfNotViolations(t, err, map[string]string{"Password": ViolationPassword})

	_, err = DeleteMultiple[PFUser](ctx, crud, DeleteMultipleOptions{Filters: hashPrefix})
	fatalIfNotViolations(t, err, map[string]string{"Password": ViolationPassword})

	fatalIfNotViolations(t, crud.ValidateFilters(&PFUser{}, hashPrefix), map[string]string{"Password": ViolationPassword})

	_, err = Get[PFPost](ctx, crud, GetOptions{Filters: sqlfilters.Filters{
		"AuthorID": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&PFUser{}, "ID", hashPrefix)},
	}})
	if !errors.Is(err, ErrSubquery) {
		t.Fatalf("Get with a password filter in a subquery returned %v, want %v", err, ErrSubquery)
	}
	fatalIfNotViolations(t, err, map[string]string{"Password": ViolationPassword})

	_, err = Get[PFPost](ctx, crud, GetOptions{Filters: sqlfilters.Filters{
		"Title": {Op: sqlfilters.OpIn, Val: sqlfilters.Subquery(&PFUser{}, "Password", nil)},
	}})
	if !errors.Is(err, ErrSubquery) || !strings.Contains(err.Error(), "Password is a password field of PFUser") {
		t.Fatalf("Get with a subquery of passwords returned %v, want %v", err, ErrSubquery)
	}
}

func testPasswordOrder(t *testing.T, crud *CRUD) {
	createPasswordFilterTables(t, crud)
	ctx := context.Background()

	_, err := Get[PFUser](ctx, crud, GetOptions{Order: []string{"Password", "asc"}})
	if !errors.Is(err, ErrInvalidOrder) || !strings.Contains(err.Error(), "Password is a password field") {
		t.Fatalf("Get ordered by a password returned %v, want %v", err, ErrInvalidOrder)
	}

	_, err = Get[PFPost](ctx, crud, GetOptions{Order: []string{"Author.Password", "desc"}})
	if !errors.Is(err, ErrInvalidOrder) || !strings.Contains(err.Error(), "Author.Password is a password field") {
		t.Fatalf("Get ordered by the password of a related object returned %v, want %v", err, ErrInvalidOrder)
	}
}

func testPasswordAggregate(t *testing.T, crud *CRUD) {
	createPasswordFilterTables(t, crud)
	ctx := context.Background()

	check := func(name string, err error, want string) {
		t.Helper()
		if !errors.Is(err, ErrAggregate) || !strings.Contains(err.Error(), want) {
			t.Fatalf("Aggregate with %s returned %v, want %v with %q", name, err, ErrAggregate, want)
		}
	}

	_, err := Aggregate[PFUser, struct {
		Password string `crud:"group:Password"`
	}](ctx, crud, AggregateOptions{})
	check("a group of passwords", err, "Password is a password field")

	_, err = Aggregate[PFUser, struct {
		Password string `crud:"agg:max:Password"`
	}](ctx, crud, AggregateOptions{})
	check("the largest password", err, "Password is a password field")

	_, err = Aggregate[PFPost, struct {
		Password string `crud:"group:Author.Password"`
	}](ctx, crud, AggregateOptions{})
	check("a group of the passwords of related objects", err, "Author.Password is a password field")
}

func TestPassword_Filters(t *testing.T)        { testPasswordFilters(t, testCRUD) }
func TestSQLite_Password_Filters(t *testing.T) { testPasswordFilters(t, testCRUDSQLite) }

func TestPassword_Order(t *testing.T)        { testPasswordOrder(t, testCRUD) }
func TestSQLite_Password_Order(t *testing.T) { testPasswordOrder(t, testCRUDSQLite) }

func TestPassword_Aggregate(t *testing.T)        { testPasswordAggregate(t, testCRUD) }
func TestSQLite_Password_Aggregate(t *testing.T) { testPasswordAggregate(t, testCRUDSQLite) }
