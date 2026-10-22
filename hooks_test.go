package gocrud

import (
	"context"
	"errors"
	"strings"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

// HookNote uses its own queries for a soft delete: deleted notes are hidden instead of removed.
type HookNote struct {
	ID     uint64
	Text   string
	Hidden bool
}

func (n *HookNote) DeleteByIDQuery() (string, error) {
	return `UPDATE "hook_note" SET "hidden" = 1 WHERE "id" = ?`, nil
}
func (n *HookNote) DeleteReturningIDQuery() (string, error) {
	return `UPDATE "hook_note" SET "hidden" = 1 RETURNING "id"`, nil
}
func (n *HookNote) SelectQuery([]string, int, int, sqlfilters.Filter) (string, error) {
	return `SELECT "id","text","hidden" FROM "hook_note" WHERE "hidden" = 0 ORDER BY "id"`, nil
}
func (n *HookNote) SelectCountQuery(sqlfilters.Filter) (string, error) {
	return `SELECT COUNT(*) FROM "hook_note" WHERE "hidden" = 0`, nil
}

func TestSQLite_ObjectQueryHooks(t *testing.T) {
	ctx := context.Background()
	crud := testCRUDSQLite
	_ = DropTable[HookNote](ctx, crud)
	if err := CreateTable[HookNote](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	for _, text := range []string{"a", "b", "c"} {
		if err := crud.Save(ctx, &HookNote{Text: text}, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	if err := crud.Delete(ctx, &HookNote{ID: 1}, DeleteOptions{}); err != nil {
		t.Fatalf("Delete failed: %s", err.Error())
	}
	notes, err := Get[HookNote](ctx, crud, GetOptions{})
	if err != nil || len(notes) != 2 || notes[0].Text != "b" {
		t.Fatalf("Get returned %v, %v, want the notes that are not hidden", notes, err)
	}
	if cnt, _ := GetCount[HookNote](ctx, crud, GetCountOptions{}); cnt != 2 {
		t.Fatalf("GetCount returned %d, want 2", cnt)
	}
	if hidden, _ := Load[HookNote](ctx, crud, 1); hidden == nil || !hidden.Hidden {
		t.Fatal("Delete did not hide the note with its own query")
	}

	if _, err := DeleteMultiple[HookNote](ctx, crud, DeleteMultipleOptions{}); err != nil {
		t.Fatalf("DeleteMultiple failed: %s", err.Error())
	}
	if cnt, _ := GetCount[HookNote](ctx, crud, GetCountOptions{}); cnt != 0 {
		t.Fatalf("GetCount returned %d after DeleteMultiple, want 0", cnt)
	}
}

// HookTag uses its own queries for the other operations, each changing the name, so that it can be seen.
type HookTag struct {
	ID   uint64
	Name string
}

var hookTagDropped int

func (t *HookTag) CreateTableQuery() (string, error) {
	return `CREATE TABLE IF NOT EXISTS "hook_tag" ("id" INTEGER PRIMARY KEY, "name" TEXT NOT NULL DEFAULT '', "source" TEXT NOT NULL DEFAULT 'hook')`, nil
}
func (t *HookTag) DropTableQuery() (string, error) {
	hookTagDropped++
	return `DROP TABLE IF EXISTS "hook_tag"`, nil
}
func (t *HookTag) InsertQuery() (string, error) {
	return `INSERT INTO "hook_tag"("name") VALUES (UPPER(?)) RETURNING "id"`, nil
}
func (t *HookTag) InsertOnConflictUpdateQuery() (string, error) {
	return `INSERT INTO "hook_tag"("id","name") VALUES (?,?) ON CONFLICT ("id") DO UPDATE SET "name"=LOWER(?)`, nil
}
func (t *HookTag) UpdateByIDQuery() (string, error) {
	return `UPDATE "hook_tag" SET "name"=? || '!' WHERE "id" = ?`, nil
}
func (t *HookTag) UpdateQuery(map[string]interface{}, sqlfilters.Filter) (string, error) {
	return `UPDATE "hook_tag" SET "name"=? || '#'`, nil
}
func (t *HookTag) SelectByIDQuery() (string, error) {
	return `SELECT "id", "name" || '*' FROM "hook_tag" WHERE "id" = ?`, nil
}

func TestSQLite_ObjectQueryHooksForSaving(t *testing.T) {
	ctx := context.Background()
	crud := testCRUDSQLite

	dropped := hookTagDropped
	if err := DropTable[HookTag](ctx, crud); err != nil || hookTagDropped != dropped+1 {
		t.Fatalf("DropTable returned %v and did not use its own query", err)
	}
	if err := CreateTable[HookTag](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	tag := &HookTag{Name: "go"}
	if err := crud.Save(ctx, tag, SaveOptions{}); err != nil {
		t.Fatalf("Save failed to insert: %s", err.Error())
	}

	var name, source string
	if err := crud.conn.QueryRowContext(ctx, `SELECT "name", "source" FROM "hook_tag" WHERE "id" = ?`, tag.ID).Scan(&name, &source); err != nil || name != "GO" || source != "hook" {
		t.Fatalf("own insert and create table queries stored %q, %q, %v, want GO and hook", name, source, err)
	}

	loaded, err := Load[HookTag](ctx, crud, tag.ID)
	if err != nil || loaded.Name != "GO*" {
		t.Fatalf("Load returned %+v, %v, want the name from its own query", loaded, err)
	}

	steps := []struct {
		save func() error
		want string
	}{
		{func() error { return crud.Save(ctx, &HookTag{ID: tag.ID, Name: "SQL"}, SaveOptions{}) }, "sql"},
		{func() error { return crud.Save(ctx, &HookTag{ID: tag.ID, Name: "db"}, SaveOptions{NoInsert: true}) }, "db!"},
		{func() error {
			return UpdateMultiple[HookTag](ctx, crud, map[string]interface{}{"Name": "orm"}, UpdateMultipleOptions{})
		}, "orm#"},
	}
	for _, step := range steps {
		if err := step.save(); err != nil {
			t.Fatalf("saving failed: %s", err.Error())
		}
		if err := crud.conn.QueryRowContext(ctx, `SELECT "name" FROM "hook_tag" WHERE "id" = ?`, tag.ID).Scan(&name); err != nil || name != step.want {
			t.Fatalf("own query stored %q, %v, want %q", name, err, step.want)
		}
	}
}

type HookWithOldSelectQuery struct {
	ID   uint64
	Text string
}

func (n *HookWithOldSelectQuery) SelectQuery([]string, int, int, *sqlfilters.Filters) (string, error) {
	return `SELECT "id","text" FROM "hook_with_old_select_query"`, nil
}

func TestQueryMethodWithOtherSignature(t *testing.T) {
	_, err := Get[HookWithOldSelectQuery](context.Background(), testCRUDSQLite, GetOptions{})
	if !errors.Is(err, QueryMethodError) || !strings.Contains(err.Error(), "SelectQuery is func([]string, int, int, *filters.Filters)") {
		t.Fatalf("Get returned err %v, want %v naming the method", err, QueryMethodError)
	}
}
