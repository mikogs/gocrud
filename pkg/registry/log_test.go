package registry

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"reflect"
	"sync"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/internal/fixtures"
	"github.com/mikogs/gocrud/pkg/gocrudtest"
)

// recordingHandler records the levels and messages of the logs.
type recordingHandler struct {
	mu      sync.Mutex
	records []string
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "reg" {
			h.records = append(h.records, record.Level.String()+" "+record.Message+" "+attr.Value.String())
		}
		return true
	})
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) take() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	records := h.records
	h.records = nil
	return records
}

func TestWithLogger(t *testing.T) {
	ctx := context.Background()
	recreateTestStructTable()
	handler := &recordingHandler{}
	svc := testService.WithLogger(slog.New(handler))
	const service = "github.com/mikogs/gocrud/pkg/registry.CRUD."

	for name, test := range map[string]struct {
		run  func(svc *CRUD) error
		want []string
	}{
		"invalid key": {func(svc *CRUD) error { _, err := svc.Read(ctx, "unknown", 1, nil); return err }, []string{"DEBUG key not found " + service + "Read"}},
		"not found":   {func(svc *CRUD) error { _, err := svc.Read(ctx, "teststruct", 999, nil); return err }, []string{"DEBUG not found " + service + "Read"}},
		"invalid filter": {func(svc *CRUD) error {
			_, err := svc.Num(ctx, "teststruct", Filters{Vals: map[string]string{"Age": "x"}})
			return err
		}, []string{"DEBUG error with validation " + service + "Num"}},
		"invalid op": {func(svc *CRUD) error {
			_, err := svc.Num(ctx, "teststruct", Filters{Vals: map[string]string{"Age": "1"}, Ops: map[string]string{"Age": "x"}})
			return err
		}, []string{"DEBUG invalid filters " + service + "Num"}},
		"invalid object": {func(svc *CRUD) error { return svc.Save(ctx, &fixtures.TestStruct{}, nil) }, []string{"DEBUG error with validation " + service + "Save"}},
		"in a transaction": {func(svc *CRUD) error {
			return svc.RunInTx(ctx, func(tx *CRUD) error { _, err := tx.Read(ctx, "teststruct", 999, nil); return err })
		}, []string{"DEBUG not found " + service + "Read"}},
		"with a transaction": {func(svc *CRUD) error {
			tx, err := testDB.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback() }()
			_, err = svc.WithTx(tx).Read(ctx, "teststruct", 999, nil)
			return err
		}, []string{"DEBUG not found " + service + "Read"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := test.run(svc); err == nil {
				t.Fatal("the operation did not fail")
			}
			if got := handler.take(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("the operation logged %v, want %v", got, test.want)
			}
		})
	}

	// an error of the database is logged as an error
	closed, err := sql.Open(gocrudtest.PostgresDriver(), "host=127.0.0.1 port=1 sslmode=disable connect_timeout=1")
	if err != nil {
		t.Fatalf("sql.Open failed: %s", err.Error())
	}
	_ = closed.Close()
	broken := New(map[string]func() any{"teststruct": func() any { return &fixtures.TestStruct{} }},
		closed, gocrud.Options{Dialect: gocrud.DialectPostgres}).WithLogger(slog.New(handler))
	if _, err := broken.Num(ctx, "teststruct", Filters{}); err == nil || errors.Is(err, gocrud.ErrNotFound) {
		t.Fatalf("Num on a closed database returned %v", err)
	}
	if got := handler.take(); !reflect.DeepEqual(got, []string{"ERROR error getting " + service + "Num"}) {
		t.Fatalf("Num on a closed database logged %v", got)
	}
}
