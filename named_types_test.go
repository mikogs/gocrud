package gocrud

import (
	"context"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type Level int

type Status string

type Flag bool

type NamedTaskCount struct {
	Done  Flag  `crud:"group:Done"`
	Tasks int64 `crud:"agg:count"`
}

type NamedTask struct {
	ID       uint64
	Level    Level
	Priority *Level
	Status   Status `validate:"oneof=open done"`
	Done     Flag
	Archived *Flag
}

func testNamedTypes(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[NamedTask](ctx, crud)
	if err := CreateTable[NamedTask](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	high := Level(9)
	for _, task := range []*NamedTask{
		{Level: 1, Status: "open"},
		{Level: 2, Priority: &high, Status: "done", Done: true},
		{Level: 3, Status: "open"},
	} {
		if err := crud.Save(ctx, task, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	loaded, err := Load[NamedTask](ctx, crud, 2)
	if err != nil || loaded.Level != 2 || *loaded.Priority != 9 || loaded.Status != "done" || !loaded.Done {
		t.Fatalf("Load returned %+v, %v", loaded, err)
	}

	for name, test := range map[string]struct {
		options GetOptions
		want    []uint64
	}{
		"typed values":    {GetOptions{Filters: sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: Status("open")}}}, []uint64{1, 3}},
		"basic values":    {GetOptions{Filters: sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "open"}, "Level": {Op: sqlfilters.OpGreater, Val: 1}}}, []uint64{3}},
		"bool":            {GetOptions{Filters: sqlfilters.Filters{"Done": {Op: sqlfilters.OpEqual, Val: true}}}, []uint64{2}},
		"in":              {GetOptions{Filters: sqlfilters.Filters{"Level": {Op: sqlfilters.OpIn, Val: []int{1, 2}}}}, []uint64{1, 2}},
		"nullable":        {GetOptions{Filters: sqlfilters.Filters{"Priority": {Op: sqlfilters.OpEqual, Val: 9}}}, []uint64{2}},
		"like":            {GetOptions{Filters: sqlfilters.Filters{"Status": {Op: sqlfilters.OpLike, Val: "done"}}}, []uint64{2}},
		"from strings":    {GetOptions{Filters: sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "open"}, "Level": {Op: sqlfilters.OpLower, Val: "3"}}, ConvertFiltersFromString: true}, []uint64{1}},
		"bool as strings": {GetOptions{Filters: sqlfilters.Filters{"Done": {Op: sqlfilters.OpEqual, Val: "true"}}, ConvertFiltersFromString: true}, []uint64{2}},
	} {
		t.Run(name, func(t *testing.T) {
			test.options.Order = []string{"ID", "asc"}
			tasks, err := Get[NamedTask](ctx, crud, test.options)
			if err != nil {
				t.Fatalf("Get failed: %s", err.Error())
			}
			var ids []uint64
			for _, task := range tasks {
				ids = append(ids, task.ID)
			}
			if !reflect.DeepEqual(ids, test.want) {
				t.Fatalf("Get returned %v, want %v", ids, test.want)
			}
		})
	}

	_, err = Get[NamedTask](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "closed"}}})
	fatalIfNotViolations(t, err, map[string]string{"Status": "oneof"})

	err = UpdateMultiple[NamedTask](ctx, crud, map[string]interface{}{"Status": "done"}, UpdateMultipleOptions{
		Filters: sqlfilters.Filters{"Level": {Op: sqlfilters.OpEqual, Val: 1}},
	})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	if task, _ := Load[NamedTask](ctx, crud, 1); task.Status != "done" {
		t.Fatalf("UpdateMultiple set Status to %q, want done", task.Status)
	}

	archived := Flag(true)
	reused := &NamedTask{Archived: &archived}
	if _, err := crud.Load(ctx, reused, 3, LoadOptions{}); err != nil || reused.Archived != nil {
		t.Fatalf("Load set Archived to %v, %v, want nil for NULL", reused.Archived, err)
	}

	counts, err := Aggregate[NamedTask, NamedTaskCount](ctx, crud, AggregateOptions{Order: []string{"Done", "asc"}})
	if err != nil || len(counts) != 2 {
		t.Fatalf("Aggregate returned %d results, %v, want 2", len(counts), err)
	}
	if *counts[0] != (NamedTaskCount{Done: false, Tasks: 2}) || *counts[1] != (NamedTaskCount{Done: true, Tasks: 1}) {
		t.Fatalf("Aggregate returned %+v and %+v, want 2 not done and 1 done", *counts[0], *counts[1])
	}

	migration, err := crud.PlanMigration(ctx, &NamedTask{})
	if err != nil || len(migration.Statements) > 0 || len(migration.Differences) > 0 {
		t.Fatalf("PlanMigration returned %+v, %v, want no changes", migration, err)
	}
}

func TestNamedTypes(t *testing.T)        { testNamedTypes(t, testCRUD) }
func TestSQLite_NamedTypes(t *testing.T) { testNamedTypes(t, testCRUDSQLite) }
