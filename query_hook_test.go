package gocrud

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type HookedItem struct {
	ID   uint64
	Name string `crud:"uniq"`
}

type hookKey struct{}

// recordingHook records the events, with the value that BeforeQuery puts in the context of the query.
type recordingHook struct {
	mu     sync.Mutex
	events []QueryEvent
	values []any
	cancel bool
}

func (h *recordingHook) BeforeQuery(ctx context.Context, event *QueryEvent) context.Context {
	ctx = context.WithValue(ctx, hookKey{}, event.Query)
	if h.cancel {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		return ctx
	}
	return ctx
}

func (h *recordingHook) AfterQuery(ctx context.Context, event *QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, *event)
	h.values = append(h.values, ctx.Value(hookKey{}))
}

func (h *recordingHook) take() []QueryEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, event := range h.events {
		if h.values[i] != event.Query {
			panic("AfterQuery did not get the context of BeforeQuery")
		}
	}
	events := h.events
	h.events, h.values = nil, nil
	return events
}

func testQueryHook(t *testing.T, db *sql.DB, dialect string) {
	ctx := context.Background()
	hook := &recordingHook{}
	crud := New(db, Options{Dialect: dialect, QueryHook: hook})
	_ = DropTable[HookedItem](ctx, crud)
	if err := CreateTable[HookedItem](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	for _, event := range hook.take() {
		if event.Op != "CreateTable" && event.Op != "DropTable" || event.Table != "hooked_item" {
			t.Fatalf("CreateTable ran the query %+v, want the operation and the table", event)
		}
	}

	item := &HookedItem{Name: "a"}
	if err := crud.Save(ctx, item, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	if events := hook.take(); len(events) != 1 || !strings.HasPrefix(events[0].Query, "INSERT INTO") || events[0].Op != "Save" ||
		events[0].Table != "hooked_item" || len(events[0].Args) != 1 || events[0].Args[0] != "a" || events[0].Err != nil || events[0].Duration <= 0 {
		t.Fatalf("Save ran the queries %+v, want one INSERT of a", events)
	}

	err := crud.Save(ctx, &HookedItem{Name: "a"}, SaveOptions{})
	var uniqErr *UniqError
	if events := hook.take(); !errors.As(err, &uniqErr) || len(events) != 1 || events[0].Err == nil {
		t.Fatalf("Save of a duplicate returned %v, with the queries %+v, want the error of the query", err, events)
	}

	if _, err := Get[HookedItem](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "a"}}}); err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if events := hook.take(); len(events) != 1 || !strings.HasPrefix(events[0].Query, "SELECT") || len(events[0].Args) != 1 || events[0].Args[0] != "a" {
		t.Fatalf("Get ran the queries %+v, want one SELECT", events)
	}

	// a row that is not found is not an error of the query
	if _, err := Load[HookedItem](ctx, crud, item.ID+1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load of a missing item returned %v, want ErrNotFound", err)
	}
	if events := hook.take(); len(events) != 1 || events[0].Err != nil {
		t.Fatalf("Load ran the queries %+v, want one without an error", events)
	}

	err = crud.RunInTx(ctx, func(tx *CRUD) error {
		if _, err := Load[HookedItem](ctx, tx, item.ID); err != nil {
			return err
		}
		return tx.Delete(ctx, item, DeleteOptions{})
	})
	if events := hook.take(); err != nil || len(events) != 4 || events[0].Query != "BEGIN" || events[0].Op != "RunInTx" ||
		events[1].Op != "Load" || events[2].Op != "Delete" || !strings.HasPrefix(events[2].Query, "DELETE") || events[3].Query != "COMMIT" {
		t.Fatalf("RunInTx returned %v, with the queries %+v, want BEGIN, a SELECT, a DELETE and COMMIT", err, events)
	}

	failed := errors.New("failed")
	err = crud.RunInTx(ctx, func(tx *CRUD) error { return failed })
	if events := hook.take(); !errors.Is(err, failed) || len(events) != 2 || events[1].Query != "ROLLBACK" || events[1].Err != nil {
		t.Fatalf("RunInTx that failed returned %v, with the queries %+v, want BEGIN and ROLLBACK", err, events)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx failed: %s", err.Error())
	}
	_, err = GetCount[HookedItem](ctx, crud.WithTx(tx), GetCountOptions{})
	_ = tx.Rollback()
	if events := hook.take(); err != nil || len(events) != 1 || events[0].Op != "GetCount" || events[0].Table != "hooked_item" {
		t.Fatalf("GetCount with WithTx returned %v, with the queries %+v, want one", err, events)
	}

	// the table is created by Migrate
	if err := DropTable[HookedItem](ctx, crud); err != nil {
		t.Fatalf("DropTable failed: %s", err.Error())
	}
	hook.take()
	if _, err := Migrate[HookedItem](ctx, crud); err != nil {
		t.Fatalf("Migrate failed: %s", err.Error())
	}
	ops := map[string]bool{}
	for _, event := range hook.take() {
		ops[event.Op] = true
		if event.Op == "Migrate" && (event.Table != "hooked_item" || !strings.HasPrefix(event.Query, "CREATE TABLE")) {
			t.Fatalf("Migrate ran the query %+v, want CREATE TABLE of hooked_item", event)
		}
	}
	if !reflect.DeepEqual(ops, map[string]bool{"RunInTx": true, "PlanMigration": true, "Migrate": true}) {
		t.Fatalf("Migrate ran the queries of the operations %v", ops)
	}

	// the query runs with the context of BeforeQuery
	hook.cancel = true
	_, err = GetCount[HookedItem](ctx, crud, GetCountOptions{})
	if events := hook.take(); !errors.Is(err, context.Canceled) || len(events) != 1 || !errors.Is(events[0].Err, context.Canceled) {
		t.Fatalf("GetCount with a cancelled context of the hook returned %v, with the queries %+v", err, events)
	}
	hook.cancel = false

	for _, name := range []string{"b", "c"} {
		if err := crud.Save(ctx, &HookedItem{Name: name}, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}
	hook.take()
	err = UpdateMultiple[HookedItem](ctx, crud, map[string]any{"Name": "b"}, UpdateMultipleOptions{})
	if events := hook.take(); err == nil || len(events) != 1 || events[0].Err == nil {
		t.Fatalf("UpdateMultiple of duplicates returned %v, with the queries %+v, want the error of the query", err, events)
	}

	if err := DropTable[HookedItem](ctx, crud); err != nil {
		t.Fatalf("DropTable failed: %s", err.Error())
	}
	hook.take()
	_, err = Get[HookedItem](ctx, crud, GetOptions{})
	if events := hook.take(); err == nil || len(events) != 1 || events[0].Err == nil {
		t.Fatalf("Get of a dropped table returned %v, with the queries %+v, want the error of the query", err, events)
	}
}

func TestQueryHook(t *testing.T)        { testQueryHook(t, testDB, DialectPostgres) }
func TestSQLite_QueryHook(t *testing.T) { testQueryHook(t, testDBSQLite, DialectSQLite) }

func TestQueryHookFunc(t *testing.T) {
	ctx := context.Background()
	_ = DropTable[HookedItem](ctx, testCRUDSQLite)
	if err := CreateTable[HookedItem](ctx, testCRUDSQLite); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	var queries []string
	crud := New(testDBSQLite, Options{Dialect: DialectSQLite, QueryHook: QueryHookFunc(func(ctx context.Context, event *QueryEvent) {
		queries = append(queries, event.Query)
	})})
	if _, err := GetCount[HookedItem](ctx, crud, GetCountOptions{}); err != nil {
		t.Fatalf("GetCount failed: %s", err.Error())
	}
	if len(queries) != 1 || !strings.HasPrefix(queries[0], "SELECT COUNT(*)") {
		t.Fatalf("QueryHookFunc was called with %v, want the query of GetCount", queries)
	}
}
