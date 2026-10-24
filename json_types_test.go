package gocrud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type Address struct {
	Street string
	City   string
}

// Point is written as [x,y], with methods with pointer receivers.
type Point struct {
	X, Y int
}

func (p *Point) MarshalJSON() ([]byte, error) { return json.Marshal([]int{p.X, p.Y}) }

func (p *Point) UnmarshalJSON(data []byte) error {
	var xy []int
	if err := json.Unmarshal(data, &xy); err != nil || len(xy) != 2 {
		return fmt.Errorf("%s is not a point", data)
	}
	p.X, p.Y = xy[0], xy[1]
	return nil
}

type JSONItem struct {
	ID       uint64
	Name     string
	Address  Address           `crud:"json"`
	Tags     []string          `crud:"json"`
	Labels   map[string]string `crud:"json"`
	Previous *Address          `crud:"json"`
	Location Point             `crud:"json"`
	Raw      json.RawMessage   `crud:"json"`
	Due      sql.NullTime
	Done     *sql.NullTime
}

type JSONItem_V1 struct {
	ID   uint64
	Name string
}

type UniqValuerItem struct {
	ID  uint64
	Ref uuid.UUID    `crud:"uniq"`
	Due sql.NullTime `crud:"uniq"`
}

func testJSONTypes(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[JSONItem](ctx, crud)
	if err := CreateTable[JSONItem_V1](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	if err := crud.Save(ctx, &JSONItem_V1{Name: "old"}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	migration, err := Migrate[JSONItem](ctx, crud)
	if err != nil || len(migration.AddedColumns) != 8 || len(migration.Differences) > 0 {
		t.Fatalf("Migrate returned %+v, %v, want 8 added columns", migration, err)
	}
	if err := crud.CheckSchema(ctx, &JSONItem{}); err != nil {
		t.Fatalf("CheckSchema failed: %s", err.Error())
	}

	old, err := Load[JSONItem](ctx, crud, 1)
	want := &JSONItem{ID: 1, Name: "old", Raw: json.RawMessage("null")}
	if err != nil || !reflect.DeepEqual(old, want) {
		t.Fatalf("Load returned %+v, %v, want %+v", old, err, want)
	}

	due := time.Date(2026, 1, 2, 3, 4, 5, 6000, time.FixedZone("CET", 3600))
	item := &JSONItem{
		Name:     "new",
		Address:  Address{Street: "Main 1", City: "Kraków"},
		Tags:     []string{"a", "b"},
		Labels:   map[string]string{"k": "it's"},
		Previous: &Address{City: "Gdańsk"},
		Location: Point{X: 1, Y: -2},
		Raw:      json.RawMessage(`{"a":[1,2]}`),
		Due:      sql.NullTime{Time: due, Valid: true},
		Done:     &sql.NullTime{Time: due.Add(time.Hour), Valid: true},
	}
	if err := crud.Save(ctx, item, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	// the times of Valuers are stored as the other times, so that they are compared in the same way
	builder, _ := crud.builder(&JSONItem{})
	storedAt := func(column string, at time.Time) int {
		var count int
		query := `SELECT COUNT(*) FROM ` + builder.QuotedTableName() + ` WHERE "` + column + `" = ` + crud.sqlDialect.Placeholder(1)
		if err := crud.conn.QueryRowContext(ctx, query, crud.TimeValue(at)).Scan(&count); err != nil {
			t.Fatalf("QueryRow failed: %s", err.Error())
		}
		return count
	}
	if storedAt("due", due) != 1 || storedAt("done", due.Add(time.Hour)) != 1 {
		t.Fatalf("Save stored Due and Done as other times than %v", crud.TimeValue(due))
	}

	reused := &JSONItem{Labels: map[string]string{"stale": "x"}}
	if _, err := crud.Load(ctx, reused, item.ID, LoadOptions{}); err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if !reflect.DeepEqual(reused.Address, item.Address) || !reflect.DeepEqual(reused.Tags, item.Tags) ||
		!reflect.DeepEqual(reused.Labels, item.Labels) || *reused.Previous != *item.Previous || reused.Location != item.Location ||
		!jsonEqual(reused.Raw, item.Raw) || !reused.Due.Valid || !reused.Due.Time.Equal(due) || !reused.Done.Time.Equal(item.Done.Time) {
		t.Fatalf("Load returned %+v, want %+v", reused, item)
	}

	err = UpdateMultiple[JSONItem](ctx, crud, map[string]interface{}{
		"Labels":   map[string]string{"x": "y"},
		"Previous": (*Address)(nil),
		"Due":      sql.NullTime{Time: due.Add(2 * time.Hour), Valid: true},
		"Done":     (*sql.NullTime)(nil),
	}, UpdateMultipleOptions{Filters: sqlfilters.Filters{"ID": {Op: sqlfilters.OpEqual, Val: item.ID}}})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	updated, err := Load[JSONItem](ctx, crud, item.ID)
	if err != nil || !reflect.DeepEqual(updated.Labels, map[string]string{"x": "y"}) || updated.Previous != nil ||
		!updated.Due.Time.Equal(due.Add(2*time.Hour)) || updated.Done != nil || updated.Location != item.Location {
		t.Fatalf("Load after UpdateMultiple returned %+v, %v", updated, err)
	}

	if storedAt("due", due.Add(2*time.Hour)) != 1 {
		t.Fatal("UpdateMultiple stored Due as another time")
	}

	err = UpdateMultiple[JSONItem](ctx, crud, map[string]interface{}{"Tags": "a"}, UpdateMultipleOptions{})
	fatalIfNotViolations(t, err, map[string]string{"Tags": ViolationType})

	_, err = Get[JSONItem](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"Address": {Op: sqlfilters.OpEqual, Val: item.Address}}})
	fatalIfNotViolations(t, err, map[string]string{"Address": ViolationType})

	items, err := Get[JSONItem](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"Previous": {Op: sqlfilters.OpEqual, Val: nil}}, Order: []string{"ID", "asc"}})
	if err != nil || len(items) != 2 {
		t.Fatalf("Get of NULL Previous returned %d items, %v, want 2", len(items), err)
	}
}

// jsonEqual compares JSON as values, as JSONB columns do not keep the spaces and the order of keys.
func jsonEqual(a, b []byte) bool {
	var aValue, bValue interface{}
	return json.Unmarshal(a, &aValue) == nil && json.Unmarshal(b, &bValue) == nil && reflect.DeepEqual(aValue, bValue)
}

func testUniqValuers(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	uniqCRUD := New(crud.db, Options{Dialect: crud.dialect, Flags: GetCountOnUniq})
	_ = DropTable[UniqValuerItem](ctx, uniqCRUD)
	if err := CreateTable[UniqValuerItem](ctx, uniqCRUD); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	due := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	ref := uuid.MustParse("0190c4a2-7b3e-7cc1-9a3f-1d2e3f405162")
	for _, item := range []*UniqValuerItem{
		{Ref: ref, Due: sql.NullTime{Time: due, Valid: true}},
		{Ref: uuid.New()},
		{Ref: uuid.New()},
	} {
		if err := uniqCRUD.Save(ctx, item, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	for field, item := range map[string]*UniqValuerItem{
		"Ref": {Ref: ref},
		"Due": {Ref: uuid.New(), Due: sql.NullTime{Time: due.In(time.FixedZone("CET", 3600)), Valid: true}},
	} {
		err := uniqCRUD.Save(ctx, item, SaveOptions{})
		var uniqErr *UniqError
		if !errors.As(err, &uniqErr) || uniqErr.Field != field {
			t.Fatalf("Save returned err %v, want an uniq error for %s", err, field)
		}
	}
}

func TestJSONTypes(t *testing.T)        { testJSONTypes(t, testCRUD) }
func TestSQLite_JSONTypes(t *testing.T) { testJSONTypes(t, testCRUDSQLite) }

func TestUniqValuers(t *testing.T)        { testUniqValuers(t, testCRUD) }
func TestSQLite_UniqValuers(t *testing.T) { testUniqValuers(t, testCRUDSQLite) }
