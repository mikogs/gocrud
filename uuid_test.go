package gocrud

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

type Ticket struct {
	ID         string
	Title      string
	CreatedAt  time.Time
	CreatedBy  uint64
	ModifiedAt time.Time
	ModifiedBy uint64
}

func recreateTicketTable(t *testing.T, crud *CRUD) {
	t.Helper()
	ctx := context.Background()

	_ = DropTable[Ticket](ctx, crud)
	if err := CreateTable[Ticket](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
}

func getTickets(t *testing.T, crud *CRUD, filter sqlfilters.Filter) []*Ticket {
	t.Helper()

	tickets, err := Get[Ticket](context.Background(), crud, GetOptions{Order: []string{"ID", "asc"}, Filters: filter})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	return tickets
}

func testUUIDGenerated(t *testing.T, crud *CRUD) {
	recreateTicketTable(t, crud)
	ctx := context.Background()

	var saved []*Ticket
	for _, title := range []string{"first", "second", "third"} {
		ticket := &Ticket{Title: title}
		if err := crud.Save(ctx, ticket, SaveOptions{ModifiedBy: 7}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}

		id, err := uuid.Parse(ticket.ID)
		if err != nil || id.Version() != 7 || id.String() != ticket.ID {
			t.Fatalf("Save set ID %q, want a UUIDv7 in the standard form", ticket.ID)
		}
		if ticket.CreatedBy != 7 || ticket.CreatedAt.IsZero() {
			t.Fatalf("Save did not set the creation fields of a new object: %+v", ticket)
		}
		saved = append(saved, ticket)
	}

	tickets := getTickets(t, crud, nil)
	if len(tickets) != 3 {
		t.Fatalf("Get returned %d tickets, want 3", len(tickets))
	}
	for i, ticket := range tickets {
		if ticket.ID != saved[i].ID || ticket.Title != saved[i].Title {
			t.Fatalf("Get returned %+v at %d, want %+v, in the order of creation", ticket, i, saved[i])
		}
	}
}

func testUUIDGiven(t *testing.T, crud *CRUD) {
	recreateTicketTable(t, crud)
	ctx := context.Background()
	id := "0190F5C3-6D1E-7B3A-9C4D-2E5F6A7B8C9D"

	ticket := &Ticket{ID: id, Title: "imported"}
	if err := crud.Save(ctx, ticket, SaveOptions{ModifiedBy: 1}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	if ticket.ID != strings.ToLower(id) {
		t.Fatalf("Save set ID %q, want it in the standard form", ticket.ID)
	}
	createdAt := ticket.CreatedAt

	if err := crud.Save(ctx, &Ticket{ID: id, Title: "updated", CreatedAt: createdAt, CreatedBy: 1}, SaveOptions{ModifiedBy: 2}); err != nil {
		t.Fatalf("Save of an existing ID failed: %s", err.Error())
	}

	tickets := getTickets(t, crud, nil)
	if len(tickets) != 1 || tickets[0].ID != ticket.ID || tickets[0].Title != "updated" || tickets[0].CreatedBy != 1 || tickets[0].ModifiedBy != 2 || !tickets[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("Get returned %+v, want the ticket updated by 2", tickets)
	}
}

func testUUIDInvalid(t *testing.T, crud *CRUD) {
	recreateTicketTable(t, crud)

	err := crud.Save(context.Background(), &Ticket{ID: "42", Title: "x"}, SaveOptions{})
	fatalIfNotViolations(t, err, map[string]string{"ID": ViolationType})
}

func testUUIDFilters(t *testing.T, crud *CRUD) {
	recreateTicketTable(t, crud)
	ctx := context.Background()

	var ids []string
	for _, title := range []string{"a", "b", "c"} {
		ticket := &Ticket{Title: title}
		if err := crud.Save(ctx, ticket, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
		ids = append(ids, ticket.ID)
	}

	tests := map[string]struct {
		filter sqlfilters.Filter
		want   []string
	}{
		"equal":  {sqlfilters.Filters{"ID": {Op: sqlfilters.OpEqual, Val: ids[1]}}, []string{"b"}},
		"in":     {sqlfilters.Filters{"ID": {Op: sqlfilters.OpIn, Val: []string{ids[0], ids[2]}}}, []string{"a", "c"}},
		"after":  {sqlfilters.Filters{"ID": {Op: sqlfilters.OpGreater, Val: ids[0]}}, []string{"b", "c"}},
		"prefix": {sqlfilters.Filters{"ID": {Op: sqlfilters.OpLike, Val: ids[2][:30] + "%"}}, []string{"c"}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var got []string
			for _, ticket := range getTickets(t, crud, test.filter) {
				got = append(got, ticket.Title)
			}
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("Get returned %v, want %v", got, test.want)
			}
		})
	}
}

func testUUIDLoadAndDelete(t *testing.T, crud *CRUD) {
	recreateTicketTable(t, crud)
	ctx := context.Background()

	ticket := &Ticket{Title: "load me"}
	if err := crud.Save(ctx, ticket, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	for _, id := range []interface{}{ticket.ID, strings.ToUpper(ticket.ID), uuid.MustParse(ticket.ID)} {
		loaded, err := Load[Ticket](ctx, crud, id)
		if err != nil || loaded.ID != ticket.ID || loaded.Title != "load me" {
			t.Fatalf("Load(%v) returned %+v, %v, want the ticket", id, loaded, err)
		}
	}

	if _, err := Load[Ticket](ctx, crud, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load of an ID that does not exist returned %v, want %v", err, ErrNotFound)
	}
	for _, id := range []interface{}{"42", 42} {
		if _, err := Load[Ticket](ctx, crud, id); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("Load(%#v) returned %v, want %v", id, err, ErrInvalidID)
		}
	}

	if err := crud.Delete(ctx, &Ticket{ID: strings.ToUpper(ticket.ID)}, DeleteOptions{}); err != nil {
		t.Fatalf("Delete failed: %s", err.Error())
	}
	if tickets := getTickets(t, crud, nil); len(tickets) != 0 {
		t.Fatalf("Delete left %+v", tickets)
	}
}

func testUUIDDeleteMultiple(t *testing.T, crud *CRUD) {
	recreateTicketTable(t, crud)
	ctx := context.Background()

	var ids []string
	for _, title := range []string{"a", "b", "c"} {
		ticket := &Ticket{Title: title}
		if err := crud.Save(ctx, ticket, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
		ids = append(ids, ticket.ID)
	}

	deleted, err := DeleteMultiple[Ticket](ctx, crud, DeleteMultipleOptions{
		Filters: sqlfilters.Filters{"ID": {Op: sqlfilters.OpIn, Val: []string{strings.ToUpper(ids[0]), ids[2]}}},
	})
	sort.Slice(deleted, func(i, j int) bool { return deleted[i].(string) < deleted[j].(string) })
	if err != nil || !reflect.DeepEqual(deleted, []interface{}{ids[0], ids[2]}) {
		t.Fatalf("DeleteMultiple returned %v, %v, want [%s %s]", deleted, err, ids[0], ids[2])
	}
}

func testUUIDInvalidFilters(t *testing.T, crud *CRUD) {
	recreateTicketTable(t, crud)

	for name, filter := range map[string]sqlfilters.Filter{
		"equal":      sqlfilters.Filters{"ID": {Op: sqlfilters.OpEqual, Val: "42"}},
		"in a group": sqlfilters.Or(sqlfilters.Filters{"Title": {Op: sqlfilters.OpEqual, Val: "a"}}, sqlfilters.Filters{"ID": {Op: sqlfilters.OpIn, Val: []string{"42"}}}),
		"number":     sqlfilters.Filters{"ID": {Op: sqlfilters.OpEqual, Val: 42}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Get[Ticket](context.Background(), crud, GetOptions{Filters: filter})
			fatalIfNotViolations(t, err, map[string]string{"ID": ViolationType})
		})
	}

	_, err := Get[Ticket](context.Background(), crud, GetOptions{
		Filters: sqlfilters.Filters{"ID": {Op: sqlfilters.OpLike, Val: "0190%"}},
	})
	if err != nil {
		t.Fatalf("Get with LIKE on the ID failed: %s", err.Error())
	}
}

func TestUUID_Generated(t *testing.T)        { testUUIDGenerated(t, testCRUD) }
func TestSQLite_UUID_Generated(t *testing.T) { testUUIDGenerated(t, testCRUDSQLite) }

func TestUUID_Given(t *testing.T)        { testUUIDGiven(t, testCRUD) }
func TestSQLite_UUID_Given(t *testing.T) { testUUIDGiven(t, testCRUDSQLite) }

func TestUUID_Invalid(t *testing.T)        { testUUIDInvalid(t, testCRUD) }
func TestSQLite_UUID_Invalid(t *testing.T) { testUUIDInvalid(t, testCRUDSQLite) }

func TestUUID_LoadAndDelete(t *testing.T)        { testUUIDLoadAndDelete(t, testCRUD) }
func TestSQLite_UUID_LoadAndDelete(t *testing.T) { testUUIDLoadAndDelete(t, testCRUDSQLite) }

func TestUUID_DeleteMultiple(t *testing.T)        { testUUIDDeleteMultiple(t, testCRUD) }
func TestSQLite_UUID_DeleteMultiple(t *testing.T) { testUUIDDeleteMultiple(t, testCRUDSQLite) }

func TestUUID_InvalidFilters(t *testing.T)        { testUUIDInvalidFilters(t, testCRUD) }
func TestSQLite_UUID_InvalidFilters(t *testing.T) { testUUIDInvalidFilters(t, testCRUDSQLite) }

func TestUUID_Filters(t *testing.T)        { testUUIDFilters(t, testCRUD) }
func TestSQLite_UUID_Filters(t *testing.T) { testUUIDFilters(t, testCRUDSQLite) }
