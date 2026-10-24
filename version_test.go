package gocrud

import (
	"context"
	"errors"
	"testing"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type VersionedDoc struct {
	ID      uint64
	Title   string
	Version int64 `crud:"version"`
}

type VersionedNote struct {
	ID      string
	Text    string
	Version uint32 `crud:"version"`
}

type VersionedOnly struct {
	ID      uint64
	Version int64 `crud:"version"`
}

type VersionAdded_V1 struct {
	ID    uint64
	Title string
}

type VersionAdded struct {
	ID      uint64
	Title   string
	Version int64 `crud:"version"`
}

func testVersion(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[VersionedDoc](ctx, crud)
	if err := CreateTable[VersionedDoc](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	// a version set by a client is not the version of a new object
	doc := &VersionedDoc{Title: "a", Version: 7}
	if err := crud.Save(ctx, doc, SaveOptions{}); err != nil || doc.Version != 1 {
		t.Fatalf("Save of a new object returned %v, with version %d, want 1", err, doc.Version)
	}
	first, _ := Load[VersionedDoc](ctx, crud, doc.ID)
	second, _ := Load[VersionedDoc](ctx, crud, doc.ID)
	if first == nil || first.Version != 1 {
		t.Fatalf("Load returned %+v, want version 1", first)
	}

	first.Title = "b"
	if err := crud.Save(ctx, first, SaveOptions{}); err != nil || first.Version != 2 {
		t.Fatalf("Save of an update returned %v, with version %d, want 2", err, first.Version)
	}
	second.Title = "c"
	if err := crud.Save(ctx, second, SaveOptions{}); !errors.Is(err, ErrVersionConflict) || second.Version != 1 {
		t.Fatalf("Save of an object changed since it was loaded returned %v, with version %d, want ErrVersionConflict and 1", err, second.Version)
	}
	if err := crud.Save(ctx, second, SaveOptions{NoInsert: true}); !errors.Is(err, ErrVersionConflict) || second.Version != 1 {
		t.Fatalf("Save with NoInsert of an object changed since it was loaded returned %v, with version %d", err, second.Version)
	}
	if stored, _ := Load[VersionedDoc](ctx, crud, doc.ID); stored.Title != "b" || stored.Version != 2 {
		t.Fatalf("Load returned %+v after the conflicts, want b with version 2", stored)
	}

	if err := crud.Save(ctx, first, SaveOptions{NoInsert: true}); err != nil || first.Version != 3 {
		t.Fatalf("Save with NoInsert returned %v, with version %d, want 3", err, first.Version)
	}
	if err := crud.Save(ctx, &VersionedDoc{ID: doc.ID + 100, Title: "x", Version: 3}, SaveOptions{NoInsert: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Save with NoInsert of a missing object returned %v, want ErrNotFound", err)
	}
	newWithNoInsert := &VersionedDoc{Title: "z", Version: 9}
	if err := crud.Save(ctx, newWithNoInsert, SaveOptions{NoInsert: true}); err != nil || newWithNoInsert.Version != 1 {
		t.Fatalf("Save with NoInsert of a new object returned %v, with version %d, want 1", err, newWithNoInsert.Version)
	}
	inserted := &VersionedDoc{ID: doc.ID + 100, Title: "y", Version: 5}
	if err := crud.Save(ctx, inserted, SaveOptions{}); err != nil || inserted.Version != 1 {
		t.Fatalf("Save of a new object with an ID returned %v, with version %d, want 1", err, inserted.Version)
	}

	err := UpdateMultiple[VersionedDoc](ctx, crud, map[string]interface{}{"Title": "d"},
		UpdateMultipleOptions{Filters: sqlfilters.Filters{"ID": {Op: sqlfilters.OpEqual, Val: doc.ID}}})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	if stored, _ := Load[VersionedDoc](ctx, crud, doc.ID); stored.Title != "d" || stored.Version != 4 {
		t.Fatalf("Load returned %+v after UpdateMultiple, want d with version 4", stored)
	}
	if err := crud.Save(ctx, first, SaveOptions{}); !errors.Is(err, ErrVersionConflict) || first.Version != 3 {
		t.Fatalf("Save of an object loaded before UpdateMultiple returned %v, with version %d, want ErrVersionConflict and 3", err, first.Version)
	}
	err = UpdateMultiple[VersionedDoc](ctx, crud, map[string]interface{}{"Version": 1}, UpdateMultipleOptions{})
	if !errors.Is(err, ErrVersionField) {
		t.Fatalf("UpdateMultiple of the version returned %v, want ErrVersionField", err)
	}

	err = crud.RunInTx(ctx, func(tx *CRUD) error {
		return tx.Save(ctx, &VersionedDoc{ID: doc.ID, Title: "e", Version: 1}, SaveOptions{NoInsert: true})
	})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("Save with NoInsert in a transaction returned %v, want ErrVersionConflict", err)
	}

	_ = DropTable[VersionedNote](ctx, crud)
	if err := CreateTable[VersionedNote](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	note := &VersionedNote{Text: "a"}
	if err := crud.Save(ctx, note, SaveOptions{}); err != nil || note.Version != 1 {
		t.Fatalf("Save of a new object with a UUID returned %v, with version %d, want 1", err, note.Version)
	}
	if err := crud.Save(ctx, note, SaveOptions{}); err != nil || note.Version != 2 {
		t.Fatalf("Save of an object with a UUID returned %v, with version %d, want 2", err, note.Version)
	}

	_ = DropTable[VersionedOnly](ctx, crud)
	if err := CreateTable[VersionedOnly](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	only := &VersionedOnly{}
	for want := int64(1); want <= 3; want++ {
		if err := crud.Save(ctx, only, SaveOptions{NoInsert: want == 3}); err != nil || only.Version != want {
			t.Fatalf("Save of an object with only a version returned %v, with version %d, want %d", err, only.Version, want)
		}
	}

	// the rows of a table get the version 0 of the added column
	_ = DropTable[VersionAdded](ctx, crud)
	if err := CreateTable[VersionAdded_V1](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	if err := crud.Save(ctx, &VersionAdded_V1{Title: "a"}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	if _, err := Migrate[VersionAdded](ctx, crud); err != nil {
		t.Fatalf("Migrate failed: %s", err.Error())
	}
	added, _ := Load[VersionAdded](ctx, crud, 1)
	if added == nil || added.Version != 0 {
		t.Fatalf("Load returned %+v, want version 0", added)
	}
	if err := crud.Save(ctx, added, SaveOptions{}); err != nil || added.Version != 1 {
		t.Fatalf("Save of an object of an added version returned %v, with version %d, want 1", err, added.Version)
	}
}

func TestVersion(t *testing.T)        { testVersion(t, testCRUD) }
func TestSQLite_Version(t *testing.T) { testVersion(t, testCRUDSQLite) }

type TwoVersions struct {
	ID    uint64
	One   int64 `crud:"version"`
	Other int64 `crud:"version"`
}

type StringVersion struct {
	ID      uint64
	Version string `crud:"version"`
}

type PointerVersion struct {
	ID      uint64
	Version *int64 `crud:"version"`
}

type IDVersion struct {
	ID   uint64 `crud:"version"`
	Name string
}

type JSONVersion struct {
	ID      uint64
	Version int64 `crud:"version json"`
}

func TestVersionField(t *testing.T) {
	for _, obj := range []interface{}{&TwoVersions{}, &StringVersion{}, &PointerVersion{}, &IDVersion{}, &JSONVersion{}} {
		if err := testCRUDSQLite.CreateTable(context.Background(), obj); !errors.Is(err, ErrVersionField) {
			t.Fatalf("CreateTable of %T returned %v, want ErrVersionField", obj, err)
		}
	}
}
