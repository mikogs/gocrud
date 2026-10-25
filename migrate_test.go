package gocrud

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

type MigAllTypes struct {
	ID          uint64
	Name        string
	Nickname    *string
	Count64     int64
	Count32     int32
	Count16     int16
	Unsigned    uint64
	Ratio64     float64
	Ratio32     float32
	Active      bool
	At          time.Time
	MaybeAt     *time.Time
	Bio         string `crud:"type:text"`
	Code        string `crud:"type:varchar(20) uniq"`
	Category    string `crud:"idx"`
	MaybeActive *bool
}

type MigCustomer struct {
	ID     uint64
	Name   string
	Code   string `crud:"type:varchar(20)"`
	Legacy int64
}

type MigCustomer_V2 struct {
	ID       uint64
	Name     *string
	Code     string  `crud:"type:varchar(50)"`
	Email    *string `crud:"uniq"`
	Points   int64   `crud:"idx"`
	JoinedAt time.Time
}

type MigCustomer_V3 struct {
	ID    uint64
	Name  string `crud:"uidx"`
	Code  string `crud:"type:varchar(20)"`
	Extra string
}

func mustMigrate(t *testing.T, crud *CRUD, obj any) *Migration {
	t.Helper()

	migration, err := crud.Migrate(context.Background(), obj)
	if err != nil {
		t.Fatalf("Migrate failed: %s", err.Error())
	}

	return migration
}

func fatalIfMigration(t *testing.T, got *Migration, want Migration) {
	t.Helper()

	sort.Strings(got.Differences)
	sort.Strings(want.Differences)
	if got.Table != want.Table || got.CreatedTable != want.CreatedTable ||
		len(got.AddedColumns)+len(want.AddedColumns) > 0 && !reflect.DeepEqual(got.AddedColumns, want.AddedColumns) ||
		len(got.CreatedIndexes)+len(want.CreatedIndexes) > 0 && !reflect.DeepEqual(got.CreatedIndexes, want.CreatedIndexes) ||
		len(got.Differences)+len(want.Differences) > 0 && !reflect.DeepEqual(got.Differences, want.Differences) {
		t.Fatalf("Migrate returned\n%+v\nwant\n%+v", *got, want)
	}
}

func testMigrateCreatesTable(t *testing.T, crud *CRUD) {
	_ = DropTable[MigAllTypes](context.Background(), crud)

	fatalIfMigration(t, mustMigrate(t, crud, &MigAllTypes{}), Migration{Table: "mig_all_types", CreatedTable: true})
	fatalIfMigration(t, mustMigrate(t, crud, &MigAllTypes{}), Migration{Table: "mig_all_types"})

	obj := &MigAllTypes{Name: "x", Code: "a", Ratio32: 1.5, At: time.Now()}
	if err := crud.Save(context.Background(), obj, SaveOptions{}); err != nil {
		t.Fatalf("Save failed in the migrated table: %s", err.Error())
	}
}

func testMigrateAddsColumns(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[MigCustomer](ctx, crud)
	mustMigrate(t, crud, &MigCustomer{})

	for _, name := range []string{"Acme", "Globex"} {
		if err := crud.Save(ctx, &MigCustomer{Name: name, Code: "C1", Legacy: 1}, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	wantDifferences := []string{
		`column "code" has type ` + map[string]string{
			DialectPostgres: "character varying(20), the field needs character varying(50)",
			DialectSQLite:   "VARCHAR(20), the field needs VARCHAR(50)",
		}[crud.dialect],
		`column "name" is NOT NULL, the field is nullable`,
		`column "legacy" is not a field of the struct`,
	}
	fatalIfMigration(t, mustMigrate(t, crud, &MigCustomer_V2{}), Migration{
		Table:          "mig_customer",
		AddedColumns:   []string{"email", "points", "joined_at"},
		CreatedIndexes: []string{"idx_mig_customer_points"},
		Differences:    wantDifferences,
	})

	customer, err := Load[MigCustomer_V2](ctx, crud, 1)
	if err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if *customer.Name != "Acme" || customer.Email != nil || customer.Points != 0 || !customer.JoinedAt.IsZero() {
		t.Fatalf("Load returned %+v, want the defaults in the added columns", *customer)
	}

	email := "office@acme.com"
	customer.Email = &email
	if err := crud.Save(ctx, customer, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	err = crud.Save(ctx, &MigCustomer_V2{Name: customer.Name, Email: &email}, SaveOptions{})
	var uniqErr *UniqError
	if !errors.As(err, &uniqErr) {
		t.Fatalf("Save returned err %v, want an uniq error for the added unique column", err)
	}

	fatalIfMigration(t, mustMigrate(t, crud, &MigCustomer_V2{}), Migration{Table: "mig_customer", Differences: wantDifferences})
}

func testMigrateRollsBack(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[MigCustomer](ctx, crud)
	mustMigrate(t, crud, &MigCustomer{})

	for i := 0; i < 2; i++ {
		if err := crud.Save(ctx, &MigCustomer{Name: "Acme"}, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	if _, err := crud.Migrate(ctx, &MigCustomer_V3{}); err == nil {
		t.Fatal("Migrate did not fail to create a unique index on duplicated values")
	}

	columns, err := crud.databaseColumns(ctx, "mig_customer")
	if err != nil {
		t.Fatalf("databaseColumns failed: %s", err.Error())
	}
	if _, ok := columns["extra"]; ok {
		t.Fatal("Migrate did not roll back the column added before it failed")
	}
}

func testMigrateInTx(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[MigCustomer](ctx, crud)
	errAbort := errors.New("abort")

	err := crud.RunInTx(ctx, func(tx *CRUD) error {
		if _, err := tx.Migrate(ctx, &MigCustomer{}); err != nil {
			return err
		}
		return errAbort
	})
	if !errors.Is(err, errAbort) {
		t.Fatalf("RunInTx returned err %v, want %v", err, errAbort)
	}

	columns, _ := crud.databaseColumns(ctx, "mig_customer")
	if len(columns) != 0 {
		t.Fatal("Migrate in a transaction that was rolled back created the table")
	}
}

func testMigrateAliasedColumns(t *testing.T, crud *CRUD) {
	_, err := Migrate[AliasProduct_WithPrice](context.Background(), crud)
	if !errors.Is(err, ErrAliasedColumnsMigrate) {
		t.Fatalf("Migrate returned err %v, want %v", err, ErrAliasedColumnsMigrate)
	}
}

func TestMigrate_CreatesTable(t *testing.T)        { testMigrateCreatesTable(t, testCRUD) }
func TestSQLite_Migrate_CreatesTable(t *testing.T) { testMigrateCreatesTable(t, testCRUDSQLite) }

func TestMigrate_AddsColumns(t *testing.T)        { testMigrateAddsColumns(t, testCRUD) }
func TestSQLite_Migrate_AddsColumns(t *testing.T) { testMigrateAddsColumns(t, testCRUDSQLite) }

func TestMigrate_RollsBack(t *testing.T)        { testMigrateRollsBack(t, testCRUD) }
func TestSQLite_Migrate_RollsBack(t *testing.T) { testMigrateRollsBack(t, testCRUDSQLite) }

func TestMigrate_InTx(t *testing.T)        { testMigrateInTx(t, testCRUD) }
func TestSQLite_Migrate_InTx(t *testing.T) { testMigrateInTx(t, testCRUDSQLite) }

func TestMigrate_AliasedColumns(t *testing.T)        { testMigrateAliasedColumns(t, testCRUD) }
func TestSQLite_Migrate_AliasedColumns(t *testing.T) { testMigrateAliasedColumns(t, testCRUDSQLite) }

type MigCustomer_V4 struct {
	ID       uint64
	Name     *string
	Code     string `crud:"type:varchar(50)"`
	Email    *string
	Points   int64
	JoinedAt time.Time
}

type MigLongIndexName struct {
	ID          uint64
	Description string `crud:"idx:a_very_long_index_name_that_goes_well_beyond_the_limit_of_postgres"`
}

func testMigrateUndeclaredIndexes(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[MigCustomer](ctx, crud)
	mustMigrate(t, crud, &MigCustomer{})
	mustMigrate(t, crud, &MigCustomer_V2{})

	if _, err := crud.conn.ExecContext(ctx, `CREATE INDEX custom_customer_code ON mig_customer (code)`); err != nil {
		t.Fatalf("creating an index failed: %s", err.Error())
	}

	migration := mustMigrate(t, crud, &MigCustomer_V4{})

	want := []string{`index "idx_mig_customer_points" is not declared in the struct`}
	// in SQLite, the unique column was added with a unique index, in PostgreSQL with a constraint
	if crud.dialect == DialectSQLite {
		want = append(want, `index "uniq_mig_customer_email" is not declared in the struct`)
	}

	var got []string
	for _, difference := range migration.Differences {
		if strings.HasPrefix(difference, "index ") {
			got = append(got, difference)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Migrate returned index differences %v, want %v", got, want)
	}
}

func testMigrateLongIndexName(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[MigLongIndexName](ctx, crud)

	mustMigrate(t, crud, &MigLongIndexName{})
	fatalIfMigration(t, mustMigrate(t, crud, &MigLongIndexName{}), Migration{Table: "mig_long_index_name"})
}

func TestMigrate_UndeclaredIndexes(t *testing.T) { testMigrateUndeclaredIndexes(t, testCRUD) }
func TestSQLite_Migrate_UndeclaredIndexes(t *testing.T) {
	testMigrateUndeclaredIndexes(t, testCRUDSQLite)
}

func TestMigrate_LongIndexName(t *testing.T)        { testMigrateLongIndexName(t, testCRUD) }
func TestSQLite_Migrate_LongIndexName(t *testing.T) { testMigrateLongIndexName(t, testCRUDSQLite) }

type MigCustomer_V5 struct {
	ID     uint64
	Name   string
	Code   string `crud:"type:varchar(20) uniq"`
	Legacy int64
}

func fatalIfUniqueDifferences(t *testing.T, migration *Migration, want []string) {
	t.Helper()

	var got []string
	for _, difference := range migration.Differences {
		if strings.Contains(difference, "unique") || strings.HasPrefix(difference, "index ") {
			got = append(got, difference)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Migrate returned differences %v, want %v", got, want)
	}
}

func testMigrateUniqueField(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[MigCustomer](ctx, crud)
	mustMigrate(t, crud, &MigCustomer{})

	for _, statement := range []string{
		`CREATE UNIQUE INDEX composite_code_name ON mig_customer (code, name)`,
		`CREATE UNIQUE INDEX partial_code ON mig_customer (code) WHERE legacy > 0`,
	} {
		if _, err := crud.conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("creating an index failed: %s", err.Error())
		}
	}

	notUnique := []string{`column "code" is not unique, the field is tagged with uniq`}
	fatalIfUniqueDifferences(t, mustMigrate(t, crud, &MigCustomer_V5{}), notUnique)

	addUnique := map[string]string{
		DialectPostgres: `ALTER TABLE mig_customer ADD CONSTRAINT mig_customer_code_key UNIQUE (code)`,
		DialectSQLite:   `CREATE UNIQUE INDEX uniq_mig_customer_code ON mig_customer (code)`,
	}[crud.dialect]
	if _, err := crud.conn.ExecContext(ctx, addUnique); err != nil {
		t.Fatalf("making the column unique failed: %s", err.Error())
	}
	fatalIfUniqueDifferences(t, mustMigrate(t, crud, &MigCustomer_V5{}), nil)
}

func TestMigrate_UniqueField(t *testing.T)        { testMigrateUniqueField(t, testCRUD) }
func TestSQLite_Migrate_UniqueField(t *testing.T) { testMigrateUniqueField(t, testCRUDSQLite) }
