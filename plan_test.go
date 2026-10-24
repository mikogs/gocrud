package gocrud

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mikogs/gocrud/internal/structinfo"
)

func mustPlanMigration(t *testing.T, crud *CRUD, obj any) *Migration {
	t.Helper()

	migration, err := crud.PlanMigration(context.Background(), obj)
	if err != nil {
		t.Fatalf("PlanMigration failed: %s", err.Error())
	}
	return migration
}

func testPlanMigrationOfNewTable(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[MigCustomer](ctx, crud)

	plan := mustPlanMigration(t, crud, &MigCustomer_V3{})
	if !plan.CreatedTable || len(plan.Statements) != 2 || !strings.HasPrefix(plan.Statements[0], `CREATE TABLE IF NOT EXISTS "mig_customer"`) ||
		!strings.HasPrefix(plan.Statements[1], `CREATE UNIQUE INDEX IF NOT EXISTS "idx_mig_customer_name"`) {
		t.Fatalf("PlanMigration returned %+v, want the table and its index created", *plan)
	}

	if again := mustPlanMigration(t, crud, &MigCustomer_V3{}); !reflect.DeepEqual(again, plan) {
		t.Fatalf("PlanMigration returned %+v after a plan, want the same plan, as nothing is changed", *again)
	}

	if migration := mustMigrate(t, crud, &MigCustomer_V3{}); !reflect.DeepEqual(migration, plan) {
		t.Fatalf("Migrate returned %+v, want the plan %+v", *migration, *plan)
	}
	fatalIfMigration(t, mustPlanMigration(t, crud, &MigCustomer_V3{}), Migration{Table: "mig_customer"})
}

func testPlanMigrationOfExistingTable(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[MigCustomer](ctx, crud)
	mustMigrate(t, crud, &MigCustomer{})

	plan := mustPlanMigration(t, crud, &MigCustomer_V2{})
	if len(plan.AddedColumns) != 3 || len(plan.CreatedIndexes) != 1 || len(plan.Differences) != 3 || len(plan.Statements) < 4 {
		t.Fatalf("PlanMigration returned %+v, want 3 columns and an index added, and 3 differences", *plan)
	}
	for _, column := range plan.AddedColumns {
		if !strings.Contains(strings.Join(plan.Statements, "\n"), `ADD COLUMN "`+column+`"`) {
			t.Fatalf("PlanMigration returned statements %v, want the column %s added", plan.Statements, column)
		}
	}

	if again := mustPlanMigration(t, crud, &MigCustomer_V2{}); !reflect.DeepEqual(again, plan) {
		t.Fatalf("PlanMigration returned %+v after a plan, want the same plan, as nothing is changed", *again)
	}

	migration := mustMigrate(t, crud, &MigCustomer_V2{})
	if !reflect.DeepEqual(migration, plan) {
		t.Fatalf("Migrate returned %+v, want the plan %+v", *migration, *plan)
	}

	after := mustPlanMigration(t, crud, &MigCustomer_V2{})
	if len(after.Statements) != 0 || !reflect.DeepEqual(after.Differences, plan.Differences) {
		t.Fatalf("PlanMigration after Migrate returned %+v, want no statements, and the same differences", *after)
	}
}

func testPlanMigrationOwnCreateTableQuery(t *testing.T, crud *CRUD) {
	_ = DropTable[HookTag](context.Background(), crud)

	plan, err := PlanMigration[HookTag](context.Background(), crud)
	query, _ := (&HookTag{}).CreateTableQuery()
	if err != nil || !plan.CreatedTable || len(plan.Statements) == 0 || !strings.HasPrefix(query, plan.Statements[0]) {
		t.Fatalf("PlanMigration returned %+v, %v, want the table created with its own query %q", plan, err, query)
	}
}

func TestPlanMigration_NewTable(t *testing.T)        { testPlanMigrationOfNewTable(t, testCRUD) }
func TestSQLite_PlanMigration_NewTable(t *testing.T) { testPlanMigrationOfNewTable(t, testCRUDSQLite) }

func TestPlanMigration_ExistingTable(t *testing.T) { testPlanMigrationOfExistingTable(t, testCRUD) }
func TestSQLite_PlanMigration_ExistingTable(t *testing.T) {
	testPlanMigrationOfExistingTable(t, testCRUDSQLite)
}

func TestSQLite_PlanMigration_OwnCreateTableQuery(t *testing.T) {
	testPlanMigrationOwnCreateTableQuery(t, testCRUDSQLite)
}

func testCheckSchema(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[MigCustomer](ctx, crud)
	_ = DropTable[MigAuthor](ctx, crud)

	err := crud.CheckSchema(ctx, &MigCustomer{}, &MigAuthor{})
	var schemaErr *SchemaError
	if !errors.As(err, &schemaErr) || !errors.Is(err, ErrSchemaDiffers) || len(schemaErr.Migrations) != 2 || !schemaErr.Migrations[0].CreatedTable {
		t.Fatalf("CheckSchema of tables that do not exist returned %v, want a *SchemaError for both tables", err)
	}

	mustMigrate(t, crud, &MigCustomer{})
	mustMigrate(t, crud, &MigAuthor{})
	if err := crud.CheckSchema(ctx, &MigCustomer{}, &MigAuthor{}); err != nil {
		t.Fatalf("CheckSchema of migrated tables returned %v, want nil", err)
	}

	err = crud.CheckSchema(ctx, &MigCustomer_V2{}, &MigAuthor{})
	if !errors.As(err, &schemaErr) || len(schemaErr.Migrations) != 1 || schemaErr.Migrations[0].Table != "mig_customer" {
		t.Fatalf("CheckSchema of a changed struct returned %v, want a *SchemaError for mig_customer only", err)
	}
	for _, want := range []string{`table "mig_customer": ALTER TABLE`, `table "mig_customer": column "legacy" is not a field of the struct`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("CheckSchema returned %q, want it to contain %q", err.Error(), want)
		}
	}

	mustMigrate(t, crud, &MigCustomer_V2{})
	err = crud.CheckSchema(ctx, &MigCustomer_V2{})
	if !errors.As(err, &schemaErr) || len(schemaErr.Migrations[0].Statements) != 0 || len(schemaErr.Migrations[0].Differences) == 0 {
		t.Fatalf("CheckSchema of a table with differences only returned %v, want a *SchemaError with the differences", err)
	}

	if err := crud.CheckSchema(ctx, &struct{ Name string }{}); !errors.Is(err, structinfo.ErrIDField) {
		t.Fatalf("CheckSchema of a struct that cannot be used returned %v, want %v", err, structinfo.ErrIDField)
	}
}

func TestCheckSchema(t *testing.T)        { testCheckSchema(t, testCRUD) }
func TestSQLite_CheckSchema(t *testing.T) { testCheckSchema(t, testCRUDSQLite) }

func TestMigrationSQL(t *testing.T) {
	migrations := []*Migration{
		{Table: "customer", Statements: []string{`ALTER TABLE "customer" ADD COLUMN "points" BIGINT`, `CREATE INDEX "idx_customer_points" ON "customer" ("points")`}, Differences: []string{`column "legacy" is not a field of the struct`}},
		{Table: "order"},
		{Table: "item", Differences: []string{`column "price" is nullable, the field is not`}},
	}

	want := `-- table "customer"
ALTER TABLE "customer" ADD COLUMN "points" BIGINT;
CREATE INDEX "idx_customer_points" ON "customer" ("points");
-- TODO: column "legacy" is not a field of the struct

-- table "item"
-- TODO: column "price" is nullable, the field is not
`
	if got := MigrationSQL(migrations...); got != want {
		t.Fatalf("MigrationSQL returned\n%s\nwant\n%s", got, want)
	}

	if got := MigrationSQL(&Migration{Table: "order"}); got != "" {
		t.Fatalf("MigrationSQL returned %q for a table without changes, want an empty string", got)
	}
}
