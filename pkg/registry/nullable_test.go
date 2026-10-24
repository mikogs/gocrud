package registry

import (
	"context"
	"testing"
	"time"

	"github.com/mikogs/gocrud"
)

type Task struct {
	ID     uint64
	Title  string
	DoneAt *time.Time
}

func TestList_NullFilters(t *testing.T) {
	ctx := context.Background()
	svc := New(map[string]func() interface{}{
		"tasks": func() interface{} { return &Task{} },
	}, testDB, gocrud.DialectPostgres)

	_ = testCRUD.DropTable(ctx, &Task{})
	if err := svc.CreateTables(ctx); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}

	doneAt := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	for _, task := range []*Task{{Title: "Done", DoneAt: &doneAt}, {Title: "Open"}} {
		if err := svc.Save(ctx, task, 0); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	for op, want := range map[string]string{"eq": "Open", "ne": "Done"} {
		objs, _, err := svc.List(ctx, "tasks", 10, 0, "", "",
			Filters{Vals: map[string]string{"DoneAt": ""}, Ops: map[string]string{"DoneAt": op}},
			nil, nil, nil)
		if err != nil {
			t.Fatalf("List failed: %s", err.Error())
		}
		if len(objs) != 1 || objs[0].(*Task).Title != want {
			t.Fatalf("List with %s and an empty value returned %+v, want %s", op, objs, want)
		}

		num, err := svc.Num(ctx, "tasks", Filters{Vals: map[string]string{"DoneAt": ""}, Ops: map[string]string{"DoneAt": op}})
		if err != nil || num != 1 {
			t.Fatalf("Num with %s and an empty value returned %d, %v, want 1", op, num, err)
		}
	}

	_, _, err := svc.List(ctx, "tasks", 10, 0, "", "",
		Filters{Vals: map[string]string{"Title": ""}, Ops: map[string]string{"Title": "eq"}}, nil, nil, nil)
	if err != nil {
		t.Fatalf("List failed for an empty value of a field that is not nullable: %s", err.Error())
	}
}
