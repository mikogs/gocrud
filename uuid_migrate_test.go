package gocrud

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mikogs/gocrud/internal/structinfo"
)

type MigAuthor struct {
	ID   string
	Name string
}

type MigTicket struct {
	ID       uint64
	Title    string
	AuthorID uint64
}

type MigTicket_V2 struct {
	ID       string
	Title    string
	AuthorID string  `crud:"fk:mig_author"`
	EditorID *string `crud:"fk:mig_author on_delete:set_null"`
}

type MigTicket_V3 struct {
	ID       uint64
	Title    string
	AuthorID string  `crud:"fk:mig_author"`
	EditorID *string `crud:"fk:mig_author on_delete:set_null"`
}

func recreateMigTicketTables(t *testing.T, crud *CRUD, ticket interface{}) {
	t.Helper()
	ctx := context.Background()

	_ = crud.DropTable(ctx, &MigTicket{})
	_ = DropTable[MigAuthor](ctx, crud)
	for _, obj := range []interface{}{&MigAuthor{}, ticket} {
		if err := crud.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed for %T: %s", obj, err.Error())
		}
	}
}

func testMigrateUUIDTables(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = crud.DropTable(ctx, &MigTicket{})
	_ = DropTable[MigAuthor](ctx, crud)

	fatalIfMigration(t, mustMigrate(t, crud, &MigAuthor{}), Migration{Table: "mig_author", CreatedTable: true})
	fatalIfMigration(t, mustMigrate(t, crud, &MigTicket_V2{}), Migration{Table: "mig_ticket", CreatedTable: true})

	fatalIfMigration(t, mustMigrate(t, crud, &MigAuthor{}), Migration{Table: "mig_author"})
	fatalIfMigration(t, mustMigrate(t, crud, &MigTicket_V2{}), Migration{Table: "mig_ticket"})
}

func testMigrateUUIDDifferences(t *testing.T, crud *CRUD) {
	recreateMigTicketTables(t, crud, &MigTicket{})

	idType, keyType, uuidType := "integer", "bigint", "uuid"
	if crud.dialect == DialectSQLite {
		idType, keyType, uuidType = "INTEGER", "INTEGER", "TEXT"
	}

	fatalIfMigration(t, mustMigrate(t, crud, &MigTicket_V2{}), Migration{
		Table:        "mig_ticket",
		AddedColumns: []string{"editor_id"},
		Differences: []string{
			`column "id" has type ` + idType + `, the ID field is a string with a UUID`,
			`column "author_id" has type ` + keyType + `, the field needs ` + uuidType,
			`column "author_id" has no foreign key, the field references table "mig_author"`,
		},
	})

	recreateMigTicketTables(t, crud, &MigTicket_V2{})
	fatalIfMigration(t, mustMigrate(t, crud, &MigTicket_V3{}), Migration{
		Table:       "mig_ticket",
		Differences: []string{`column "id" has type ` + uuidType + `, the ID field is a uint64`},
	})
}

func TestMigrate_UUIDTables(t *testing.T) { testMigrateUUIDTables(t, testCRUD) }
func TestSQLite_Migrate_UUIDTables(t *testing.T) {
	testMigrateUUIDTables(t, newSQLiteForeignKeysCRUD(t))
}

func TestMigrate_UUIDDifferences(t *testing.T) { testMigrateUUIDDifferences(t, testCRUD) }
func TestSQLite_Migrate_UUIDDifferences(t *testing.T) {
	testMigrateUUIDDifferences(t, newSQLiteForeignKeysCRUD(t))
}

type StringAuditNote struct {
	ID         string
	Text       string
	CreatedAt  time.Time
	CreatedBy  string
	ModifiedAt time.Time
	ModifiedBy string
}

func testStringModificationFields(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[StringAuditNote](ctx, crud)
	if err := CreateTable[StringAuditNote](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	note := &StringAuditNote{Text: "a"}
	if err := crud.Save(ctx, note, SaveOptions{ModifiedBy: "auth0|42"}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	if err := crud.Save(ctx, &StringAuditNote{ID: note.ID, Text: "b", CreatedAt: note.CreatedAt, CreatedBy: "forged"}, SaveOptions{ModifiedBy: "auth0|7"}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	loaded, err := Load[StringAuditNote](ctx, crud, note.ID)
	if err != nil || loaded.Text != "b" || loaded.CreatedBy != "auth0|42" || loaded.ModifiedBy != "auth0|7" {
		t.Fatalf("Load returned %+v, %v, want the note created by auth0|42 and modified by auth0|7", loaded, err)
	}

	if err := crud.Save(ctx, &StringAuditNote{Text: "c"}, SaveOptions{ModifiedBy: 42}); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Save with a number for a string field returned %v, want %v", err, ErrInvalidID)
	}
}

func TestStringModificationFields(t *testing.T) { testStringModificationFields(t, testCRUD) }
func TestSQLite_StringModificationFields(t *testing.T) {
	testStringModificationFields(t, testCRUDSQLite)
}

func TestModificationFieldsUserIDs(t *testing.T) {
	ctx := context.Background()
	recreateTicketTable(t, testCRUDSQLite)

	for _, by := range []interface{}{uint64(7), 7, "7"} {
		ticket := &Ticket{Title: "a"}
		if err := testCRUDSQLite.Save(ctx, ticket, SaveOptions{ModifiedBy: by}); err != nil || ticket.CreatedBy != 7 || ticket.ModifiedBy != 7 {
			t.Fatalf("Save with ModifiedBy %#v returned %v and set %d, %d, want 7", by, err, ticket.CreatedBy, ticket.ModifiedBy)
		}
	}

	ticket := &Ticket{Title: "a"}
	if err := testCRUDSQLite.Save(ctx, ticket, SaveOptions{}); err != nil || ticket.CreatedBy != 0 {
		t.Fatalf("Save without ModifiedBy returned %v and set %d, want 0", err, ticket.CreatedBy)
	}

	if err := testCRUDSQLite.Save(ctx, &Ticket{Title: "a"}, SaveOptions{ModifiedBy: "auth0|7"}); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Save with a text for a uint64 field returned %v, want %v", err, ErrInvalidID)
	}

	type mixedBy struct {
		ID         uint64
		CreatedAt  time.Time
		CreatedBy  string
		ModifiedAt time.Time
		ModifiedBy uint64
	}
	if err := testCRUDSQLite.Save(ctx, &mixedBy{}, SaveOptions{}); !errors.Is(err, structinfo.ErrModificationFields) {
		t.Fatalf("Save of a struct with CreatedBy and ModifiedBy of different types returned %v, want %v", err, structinfo.ErrModificationFields)
	}
}
