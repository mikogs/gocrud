package gocrud

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type BatchUser struct {
	ID         uint64
	Email      string `crud:"uniq" validate:"required"`
	Password   string `crud:"pass"`
	CreatedAt  time.Time
	CreatedBy  uint64
	ModifiedAt time.Time
	ModifiedBy uint64
	Version    int64 `crud:"version"`
}

type BatchNote struct {
	ID   string
	Text string
}

// batchUsers returns n users, of which only the first has a password, as bcrypt takes long.
func batchUsers(n int) []*BatchUser {
	users := make([]*BatchUser, n)
	for i := range users {
		users[i] = &BatchUser{Email: fmt.Sprintf("user%d@example.com", i)}
	}
	users[0].Password = "secret"
	return users
}

func testInsertMultiple(t *testing.T, db *sql.DB, dialect string) {
	ctx := context.Background()
	var queries []string
	crud := New(db, Options{Dialect: dialect, QueryHook: QueryHookFunc(func(_ context.Context, event *QueryEvent) {
		queries = append(queries, event.Query)
	})})
	_ = DropTable[BatchUser](ctx, crud)
	if err := CreateTable[BatchUser](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	if err := InsertMultiple[BatchUser](ctx, crud, nil, InsertMultipleOptions{}); err != nil {
		t.Fatalf("InsertMultiple of no objects failed: %s", err.Error())
	}

	// more than the rows of one query
	users := batchUsers(2500)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	queries = nil
	if err := InsertMultiple(ctx, crud, users, InsertMultipleOptions{ModifiedBy: uint64(7), ModifiedAt: at}); err != nil {
		t.Fatalf("InsertMultiple failed: %s", err.Error())
	}
	inserts := 0
	for _, query := range queries {
		if strings.HasPrefix(query, "INSERT INTO") {
			inserts++
		}
	}
	if inserts != 3 {
		t.Fatalf("InsertMultiple ran %d INSERT queries, want 3: %v", inserts, len(queries))
	}
	for i, user := range users {
		if user.ID == 0 || i > 0 && user.ID <= users[i-1].ID || user.Password != "" || user.Version != 1 ||
			!user.CreatedAt.Equal(at) || user.CreatedBy != 7 || user.ModifiedBy != 7 {
			t.Fatalf("InsertMultiple set the object %d to %+v", i, user)
		}
	}
	stored, err := Get[BatchUser](ctx, crud, GetOptions{})
	if err != nil || len(stored) != len(users) {
		t.Fatalf("Get returned %d users, %v, want %d", len(stored), err, len(users))
	}
	emails := map[uint64]string{}
	for _, user := range stored {
		emails[user.ID] = user.Email
	}
	for i, user := range users {
		if emails[user.ID] != user.Email {
			t.Fatalf("the object %d got the ID %d of %s", i, user.ID, emails[user.ID])
		}
	}
	verified, err := crud.VerifyPasswords(ctx, &BatchUser{}, VerifyPasswordsOptions{
		Filters:   sqlfilters.Filters{"ID": {Op: sqlfilters.OpEqual, Val: users[0].ID}},
		Passwords: map[string]string{"Password": "secret"},
	})
	if err != nil || !verified {
		t.Fatalf("VerifyPasswords returned %t, %v, want the password verified", verified, err)
	}

	failing := map[string]struct {
		users []*BatchUser
		check func(err error) bool
	}{
		"invalid": {[]*BatchUser{{Email: "new1@example.com"}, {}}, func(err error) bool {
			var objErr *ObjectError
			var validationErr *ValidationError
			return errors.As(err, &objErr) && objErr.Index == 1 && errors.As(err, &validationErr) && validationErr.Violations["Email"] == "required"
		}},
		"duplicate": {[]*BatchUser{{Email: "new2@example.com", Password: "a"}, {Email: users[0].Email, Password: "b"}}, func(err error) bool {
			var uniqErr *UniqError
			return errors.As(err, &uniqErr) && uniqErr.Field == "Email"
		}},
		"with an ID": {[]*BatchUser{{Email: "new3@example.com"}, {ID: 99999, Email: "new4@example.com"}}, func(err error) bool {
			var objErr *ObjectError
			return errors.As(err, &objErr) && objErr.Index == 1 && errors.Is(err, ErrInvalidID)
		}},
	}
	for name, test := range failing {
		t.Run(name, func(t *testing.T) {
			before := make([]BatchUser, len(test.users))
			for i, user := range test.users {
				before[i] = *user
			}
			err := InsertMultiple(ctx, crud, test.users, InsertMultipleOptions{})
			if !test.check(err) {
				t.Fatalf("InsertMultiple returned %v", err)
			}
			for i, user := range test.users {
				if *user != before[i] {
					t.Fatalf("InsertMultiple left the object %d as %+v, want %+v", i, *user, before[i])
				}
			}
			if count, _ := GetCount[BatchUser](ctx, crud, GetCountOptions{}); count != uint64(len(users)) {
				t.Fatalf("GetCount returned %d after a failed InsertMultiple, want %d", count, len(users))
			}
		})
	}

	err = crud.InsertMultiple(ctx, []any{&BatchUser{Email: "a"}, &BatchNote{Text: "a"}}, InsertMultipleOptions{})
	if !errors.Is(err, ErrInsertMultiple) {
		t.Fatalf("InsertMultiple of different types returned %v, want ErrInsertMultiple", err)
	}

	err = crud.RunInTx(ctx, func(tx *CRUD) error {
		if err := InsertMultiple(ctx, tx, []*BatchUser{{Email: "rolled-back@example.com"}}, InsertMultipleOptions{}); err != nil {
			return err
		}
		return errors.New("rolled back")
	})
	if count, _ := GetCount[BatchUser](ctx, crud, GetCountOptions{}); err == nil || count != uint64(len(users)) {
		t.Fatalf("InsertMultiple in a rolled back transaction returned %v, with %d users, want %d", err, count, len(users))
	}

	_ = DropTable[BatchNote](ctx, crud)
	if err := CreateTable[BatchNote](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	given := "0190c4a2-7b3e-7cc1-9a3f-1d2e3f405161"
	notes := []*BatchNote{{Text: "a"}, {ID: strings.ToUpper(given), Text: "b"}}
	if err := InsertMultiple(ctx, crud, notes, InsertMultipleOptions{}); err != nil {
		t.Fatalf("InsertMultiple of objects with UUIDs failed: %s", err.Error())
	}
	if len(notes[0].ID) != 36 || notes[1].ID != given {
		t.Fatalf("InsertMultiple set the IDs %s and %s, want a new UUID and %s", notes[0].ID, notes[1].ID, given)
	}
	if note, err := Load[BatchNote](ctx, crud, given); err != nil || note.Text != "b" {
		t.Fatalf("Load returned %+v, %v, want the note b", note, err)
	}
}

func TestInsertMultiple(t *testing.T)        { testInsertMultiple(t, testDB, DialectPostgres) }
func TestSQLite_InsertMultiple(t *testing.T) { testInsertMultiple(t, testDBSQLite, DialectSQLite) }

func TestSQLite_InsertMultipleWithInsertQuery(t *testing.T) {
	ctx := context.Background()
	crud := testCRUDSQLite
	_ = DropTable[HookTag](ctx, crud)
	if err := CreateTable[HookTag](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	tags := []*HookTag{{Name: "a"}, {Name: "b"}}
	if err := InsertMultiple(ctx, crud, tags, InsertMultipleOptions{}); err != nil {
		t.Fatalf("InsertMultiple failed: %s", err.Error())
	}
	for i, want := range []string{"A", "B"} {
		var name string
		if err := crud.conn.QueryRowContext(ctx, `SELECT "name" FROM "hook_tag" WHERE "id" = ?`, tags[i].ID).Scan(&name); err != nil || name != want {
			t.Fatalf("the tag %d was stored as %q, %v, want %s with its own insert query", i, name, err, want)
		}
	}
}

func TestSQLite_InsertMultipleWithSkippedRows(t *testing.T) {
	ctx := context.Background()
	crud := testCRUDSQLite
	_ = DropTable[BatchUser](ctx, crud)
	if err := CreateTable[BatchUser](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	trigger := `CREATE TRIGGER "skip_batch_user" BEFORE INSERT ON "batch_user" WHEN NEW."email" = 'skip' BEGIN SELECT RAISE(IGNORE); END`
	if _, err := crud.conn.ExecContext(ctx, trigger); err != nil {
		t.Fatalf("CREATE TRIGGER failed: %s", err.Error())
	}
	users := []*BatchUser{{Email: "a"}, {Email: "skip"}, {Email: "b"}}
	if err := InsertMultiple(ctx, crud, users, InsertMultipleOptions{}); err == nil || users[0].ID != 0 {
		t.Fatalf("InsertMultiple with a skipped row returned %v, with the ID %d, want an error and no IDs", err, users[0].ID)
	}
	if count, _ := GetCount[BatchUser](ctx, crud, GetCountOptions{}); count != 0 {
		t.Fatalf("GetCount returned %d after a failed InsertMultiple, want 0", count)
	}
}
