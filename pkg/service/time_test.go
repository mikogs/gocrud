package service

import (
	"context"
	"testing"
	"time"

	"github.com/mikogs/gocrud"
)

type Event struct {
	ID       uint64
	Name     string
	StartsAt time.Time
}

func TestList_TimeFilters(t *testing.T) {
	ctx := context.Background()
	svc := New(map[string]func() interface{}{
		"events": func() interface{} { return &Event{} },
	}, testDB, gocrud.DialectPostgres)

	_ = testCRUD.DropTable(ctx, &Event{})
	if err := svc.CreateTables(ctx); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}

	for _, event := range []*Event{
		{Name: "Past", StartsAt: time.Date(2024, 4, 30, 9, 0, 0, 0, time.UTC)},
		{Name: "Today", StartsAt: time.Date(2024, 5, 1, 9, 0, 0, 0, time.UTC)},
		{Name: "Future", StartsAt: time.Date(2024, 5, 2, 9, 0, 0, 0, time.UTC)},
	} {
		if err := svc.Save(ctx, event, 0); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	objs, _, err := svc.List(ctx, "events", 10, 0, "StartsAt", "asc",
		map[string]string{"StartsAt": "2024-05-01"},
		map[string]string{"StartsAt": "ge"},
		nil, nil, nil)
	if err != nil {
		t.Fatalf("List failed: %s", err.Error())
	}
	if len(objs) != 2 || objs[0].(*Event).Name != "Today" || objs[1].(*Event).Name != "Future" {
		t.Fatalf("List returned %+v, want Today and Future", objs)
	}

	_, _, err = svc.List(ctx, "events", 10, 0, "", "",
		map[string]string{"StartsAt": "May 1st"},
		map[string]string{"StartsAt": "ge"},
		nil, nil, nil)
	fatalIfNotFilterViolations(t, err, map[string]string{"StartsAt": gocrud.ViolationType})
}
