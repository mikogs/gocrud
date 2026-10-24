package registry

import (
	"context"
	"strings"
	"testing"

	"github.com/mikogs/gocrud"
)

func TestNew_Options(t *testing.T) {
	ctx := context.Background()
	var queries []string
	svc := New(map[string]func() interface{}{
		"events": func() interface{} { return &Event{} },
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres, TableNamePrefix: "opt_", QueryHook: gocrud.QueryHookFunc(func(_ context.Context, event *gocrud.QueryEvent) {
		queries = append(queries, event.Query)
	})})

	if err := svc.CreateTables(ctx); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}
	if len(queries) == 0 || !strings.Contains(queries[0], `"opt_event"`) {
		t.Fatalf("CreateTables ran %v, want the queries of the table with the prefix, through the hook", queries)
	}
	if _, err := testDB.ExecContext(ctx, `DROP TABLE "opt_event"`); err != nil {
		t.Fatalf("DROP TABLE failed: %s", err.Error())
	}
}
