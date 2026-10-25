package registry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mikogs/gocrud"
)

type Issue struct {
	ID         string
	Title      string
	CreatedAt  time.Time
	CreatedBy  string
	ModifiedAt time.Time
	ModifiedBy string
}

func issuesService(t *testing.T) *CRUD {
	t.Helper()
	ctx := context.Background()

	svc := New(map[string]func() any{
		"issues": func() any { return &Issue{} },
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})

	_ = testCRUD.DropTable(ctx, &Issue{})
	if err := svc.CreateTables(ctx); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}

	return svc
}

func TestUUID_SaveReadDelete(t *testing.T) {
	svc := issuesService(t)
	ctx := context.Background()

	issue := &Issue{Title: "broken"}
	if err := svc.Save(ctx, issue, "auth0|42"); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	id, ok := svc.ID(issue).(string)
	if _, err := uuid.Parse(id); !ok || err != nil || issue.CreatedBy != "auth0|42" {
		t.Fatalf("Save set ID %#v and CreatedBy %q, want a UUID and auth0|42", svc.ID(issue), issue.CreatedBy)
	}

	for _, readID := range []any{id, strings.ToUpper(id)} {
		obj, err := svc.Read(ctx, "issues", readID, nil)
		if err != nil || obj.(*Issue).Title != "broken" {
			t.Fatalf("Read(%v) returned %+v, %v, want the issue", readID, obj, err)
		}
	}

	if _, err := svc.Read(ctx, "issues", uuid.NewString(), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Read of an ID that does not exist returned %v, want %v", err, ErrNotFound)
	}
	for _, invalidID := range []any{"42", 42} {
		if _, err := svc.Read(ctx, "issues", invalidID, nil); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("Read(%#v) returned %v, want %v", invalidID, err, ErrInvalidID)
		}
		if err := svc.Delete(ctx, "issues", invalidID); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("Delete(%#v) returned %v, want %v", invalidID, err, ErrInvalidID)
		}
	}

	if err := svc.Save(ctx, &Issue{Title: "x"}, 42); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Save with a number for a string user ID returned %v, want %v", err, ErrInvalidID)
	}

	if err := svc.Delete(ctx, "issues", strings.ToUpper(id)); err != nil {
		t.Fatalf("Delete failed: %s", err.Error())
	}
	if num, err := svc.Num(ctx, "issues", Filters{}); err != nil || num != 0 {
		t.Fatalf("Num returned %d, %v, want the issue deleted", num, err)
	}
}

func TestUUID_ListByID(t *testing.T) {
	svc := issuesService(t)
	ctx := context.Background()

	var ids []string
	for _, title := range []string{"a", "b", "c"} {
		issue := &Issue{Title: title}
		if err := svc.Save(ctx, issue, ""); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
		ids = append(ids, issue.ID)
	}

	objs, err := svc.List(ctx, "issues", ListOptions{Limit: 10, Order: "ID", OrderDirection: "asc", Filters: Filters{
		Vals: map[string]string{"ID": strings.ToUpper(ids[0]) + "," + ids[2]},
		Ops:  map[string]string{"ID": "in"},
	}})
	if err != nil || len(objs) != 2 || objs[0].(*Issue).Title != "a" || objs[1].(*Issue).Title != "c" {
		t.Fatalf("List returned %+v, %v, want the issues a and c", objs, err)
	}

	_, err = svc.List(ctx, "issues", ListOptions{Limit: 10, Filters: Filters{Vals: map[string]string{"ID": "42"}}})
	fatalIfNotFilterViolations(t, err, map[string]string{"ID": gocrud.ViolationType})
}
