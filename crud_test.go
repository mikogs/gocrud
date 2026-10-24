package gocrud

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud/internal/fixtures"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

func TestPasswordFieldsFor(t *testing.T) {
	if got := testCRUDSQLite.PasswordFieldsFor(&PasswordStruct{}); !reflect.DeepEqual(got, []string{"Password"}) {
		t.Fatalf("PasswordFieldsFor returned %v, want [Password]", got)
	}
	if got := testCRUDSQLite.PasswordFieldsFor(&fixtures.TestStruct{}); len(got) != 0 {
		t.Fatalf("PasswordFieldsFor returned %v for a struct without password fields, want none", got)
	}
	if got := testCRUDSQLite.PasswordFieldsFor(nil); got != nil {
		t.Fatalf("PasswordFieldsFor returned %v for an invalid object, want nil", got)
	}
}

func TestNew_InvalidTableNamePrefix(t *testing.T) {
	for _, prefix := range []string{`app"; DROP TABLE x; --`, "App_", "app-", "app."} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("New with the table name prefix %q did not panic", prefix)
				}
			}()
			New(testDBSQLite, Options{Dialect: DialectSQLite, TableNamePrefix: prefix})
		}()
	}

	New(testDBSQLite, Options{Dialect: DialectSQLite, TableNamePrefix: "app_2_"})
}

func TestRelatedType(t *testing.T) {
	tests := []struct {
		obj   interface{}
		field string
		want  reflect.Type
	}{
		{&PFPost{}, "Author", reflect.TypeOf(PFUser{})},
		{&UAuthor{}, "Posts", reflect.TypeOf(UPost{})},
		{&PFPost{}, "Title", nil},
		{&PFPost{}, "Unknown", nil},
	}
	for _, test := range tests {
		got, ok := testCRUD.RelatedType(test.obj, test.field)
		if got != test.want || ok != (test.want != nil) {
			t.Fatalf("RelatedType of %T.%s returned %v, %v, want %v", test.obj, test.field, got, ok, test.want)
		}
	}
}

type WithoutIDField struct {
	Name string
}

func TestErrors_OfOperations(t *testing.T) {
	ctx := context.Background()
	recreatePasswordTable(t, testCRUDSQLite)

	_, err := Get[WithoutIDField](ctx, testCRUDSQLite, GetOptions{})
	if !errors.Is(err, ErrIDField) {
		t.Fatalf("Get of a struct without an ID returned %v, want %v", err, ErrIDField)
	}

	for want, filter := range map[error]sqlfilters.Filter{
		ErrRawFilter:      sqlfilters.Raw(".Name = ? AND .ID = ?", "x"),
		ErrMatchFilter:    sqlfilters.Filters{"Name": {Op: sqlfilters.OpMatch, Val: "^a"}},
		ErrPasswordFilter: sqlfilters.Raw(".Password LIKE ?", "$2a%"),
	} {
		if _, err := Get[PasswordStruct](ctx, testCRUDSQLite, GetOptions{Filters: filter}); !errors.Is(err, want) {
			t.Fatalf("Get with %v returned %v, want %v", filter, err, want)
		}
	}
}
