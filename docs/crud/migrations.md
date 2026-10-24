# Migrations

`CreateTable` creates a table only when it does not exist. When you add a field to a struct later, the table does not change, and saving the struct fails because of the missing column.

`Migrate` updates an existing table to match the struct, but only with changes that cannot lose data:

* It creates the table and its indexes when the table does not exist.
* It adds the columns of new fields. Existing rows get the default value of the column, or `NULL` for a pointer field.
* It creates new indexes declared with `idx` and `uidx`.

Everything else is reported, but not changed. See [Changes that are not made](#changes-that-are-not-made).

## Migrate

```go
func (c *CRUD) Migrate(ctx context.Context, obj interface{}) (*Migration, error)
func Migrate[T any](ctx context.Context, c *CRUD) (*Migration, error)
```

```go
type Migration struct {
    Table          string   // the name of the table
    CreatedTable   bool     // the table did not exist and was created
    AddedColumns   []string // the names of the columns that were added
    CreatedIndexes []string // the names of the indexes that were created
    Statements     []string // the SQL statements that made the changes, in their order
    Differences    []string // the differences that were not changed
}
```

All the changes are made in a transaction. If any of them fails, e.g. a unique index cannot be created because some rows have the same value, nothing is changed and an error is returned. When `Migrate` is called in a [transaction](transactions.md), it is part of that transaction.

A good place to run it is the start of the application, before it serves requests:

```go
func migrate(ctx context.Context, crud *gocrud.CRUD) error {
    for _, obj := range []interface{}{&User{}, &Order{}, &OrderItem{}, &Product{}} {
        migration, err := crud.Migrate(ctx, obj)
        if err != nil {
            return fmt.Errorf("migrating %T: %w", obj, err)
        }

        if migration.CreatedTable {
            slog.Info("created table", slog.String("table", migration.Table))
        }
        for _, column := range migration.AddedColumns {
            slog.Info("added column", slog.String("table", migration.Table), slog.String("column", column))
        }
        for _, difference := range migration.Differences {
            slog.Warn("table differs from struct", slog.String("table", migration.Table), slog.String("difference", difference))
        }
    }
    return nil
}
```

If several instances of the application start at the same time, run the migration from one place only, e.g. a separate command run before the deployment. Two instances adding the same column at the same time make one of them fail.

## Planning a migration

`PlanMigration` returns the same `Migration` as `Migrate`, without changing anything, so that the changes can be checked first, or written to the file of a migration tool:

```go
func (c *CRUD) PlanMigration(ctx context.Context, obj interface{}) (*Migration, error)
func PlanMigration[T any](ctx context.Context, c *CRUD) (*Migration, error)
```

```go
plan, err := gocrud.PlanMigration[Customer](ctx, crud)
if err != nil {
    return err
}
for _, statement := range plan.Statements {
    fmt.Println(statement + ";")
}
// in PostgreSQL, for the new fields Email *string `crud:"uniq"` and Points int64 `crud:"idx"`:
// ALTER TABLE "customer" ADD COLUMN "email" VARCHAR(255) UNIQUE;
// ALTER TABLE "customer" ADD COLUMN "points" BIGINT NOT NULL DEFAULT 0;
// CREATE INDEX IF NOT EXISTS "idx_customer_points" ON "customer" ("points");
```

`MigrationSQL` formats migrations as an SQL script, e.g. for the file of a migration tool. The differences, which have to be resolved by hand, are added as `-- TODO:` comments, and migrations without changes are left out:

```go
plans, err := reg.PlanMigrations(ctx)
if err != nil {
    return err
}
fmt.Print(gocrud.MigrationSQL(plans...))
// -- table "customer"
// ALTER TABLE "customer" ADD COLUMN "points" BIGINT NOT NULL DEFAULT 0;
// -- TODO: column "legacy" is not a field of the struct
```

See [Versioned migrations](versioned-migrations.md) for a complete workflow with a migration tool.

`Migrate` runs exactly the planned statements. The statements are planned for the database as it is at the time, so run them in their order, and before the database changes in another way. They are also specific to the database, so plan with the same kind of database, PostgreSQL or SQLite, as the one where they are run.

## Checking the schema

`CheckSchema` checks that the tables match the structs: that `Migrate` would not change them, and would not report any differences.

```go
func (c *CRUD) CheckSchema(ctx context.Context, objs ...interface{}) error
```

When a table does not match, it returns a `*gocrud.SchemaError`, which matches `gocrud.ErrSchemaDiffers`. Its `Migrations` are the plans of the tables that do not match, and its message lists their statements and differences, e.g.:

```
database schema differs from the structs: table "customer": ALTER TABLE "customer" ADD COLUMN "points" BIGINT NOT NULL DEFAULT 0; table "customer": column "legacy" is not a field of the struct
```

It is meant for applications whose tables are changed by a migration tool instead of `Migrate`. A test can check that the migration files are complete, i.e. that a field was not added to a struct without a migration:

```go
func TestSchema(t *testing.T) {
    // the database of the test, with the migration files applied
    if err := crud.CheckSchema(context.Background(), &User{}, &Order{}, &OrderItem{}); err != nil {
        t.Fatal(err)
    }
}
```

At the start of an application, it stops a version that does not match the database from running:

```go
if err := crud.CheckSchema(ctx, &User{}, &Order{}, &OrderItem{}); err != nil {
    log.Fatalf("run the migrations first: %v", err)
}
```

The differences include the ones that are not about a table, e.g. that SQLite does not enforce foreign keys, and the ones that are fine for you, e.g. a column that is used only by another application. Such a table does not pass the check, so `CheckSchema` fits structs that describe their tables completely.

## Example

Version 1 of a struct:

```go
type Customer struct {
    ID   uint64 `json:"id"`
    Name string `json:"name" validate:"required"`
    Code string `json:"code" crud:"type:varchar(20)"`
}
```

Version 2 adds an optional, unique email, a number of loyalty points with an index, and a longer code:

```go
type Customer struct {
    ID     uint64  `json:"id"`
    Name   string  `json:"name" validate:"required"`
    Code   string  `json:"code" crud:"type:varchar(50)"`
    Email  *string `json:"email" crud:"uniq"`
    Points int64   `json:"points" crud:"idx"`
}
```

`Migrate` on version 2 returns:

```
Table:          customer
AddedColumns:   [email points]
CreatedIndexes: [idx_customer_points]
Differences:    [column "code" has type character varying(20), the field needs character varying(50)]
```

The existing customers have `NULL` emails and `0` points. The longer code has to be changed by hand.

## Changes that are not made

`Migrate` never drops, renames or changes columns. These changes can lose data, or need a decision that only you can make:

| Difference | Why it is not changed |
|---|---|
| A column that is not a field of the struct | It may be a field that was renamed, or data still used elsewhere. Dropping it would lose the data. |
| A column of a different type | The existing values may not fit the new type, e.g. text in a number column, or a longer text in a shorter column. |
| An `ID` column of the other kind, e.g. `SERIAL` for a `string` ID with a UUID | The IDs are referenced by other tables and by URLs, so they have to be converted together, with a plan. The exact type of a numeric `ID` column is not compared, as it depends on how the table was created. |
| A `NOT NULL` column of a pointer field | Usually safe, but it changes what other code reading the table can expect. |
| A nullable column of a field that is not a pointer | Existing `NULL` values have to be replaced first. |
| An existing column of a field tagged with `uniq` that is not unique | The column may already have duplicate values, which have to be resolved first. |
| An index that is no longer declared with `idx` or `uidx` | Other code or reports may still rely on it, and dropping a unique index removes a check of the data. |

They are listed in `Migration.Differences`, so that you can log them, or fail a check in CI with [`CheckSchema`](#checking-the-schema). Make these changes with SQL, or with a migration tool, see [Versioned migrations](versioned-migrations.md).

For example, in PostgreSQL:

```sql
-- a renamed field: rename the column instead of adding a new one
ALTER TABLE customer RENAME COLUMN phone TO phone_number;

-- a longer text
ALTER TABLE customer ALTER COLUMN code TYPE VARCHAR(50);

-- a pointer field: allow NULL and remove the default
ALTER TABLE customer ALTER COLUMN nickname DROP NOT NULL, ALTER COLUMN nickname DROP DEFAULT;

-- a field tagged with uniq, after removing the duplicate values
ALTER TABLE customer ADD CONSTRAINT customer_code_key UNIQUE (code);

-- CreatedAt changed from a Unix timestamp to time.Time
ALTER TABLE customer
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING to_timestamp(created_at),
    ALTER COLUMN created_at SET DEFAULT '0001-01-01 00:00:00+00';
```

Rename the column before running `Migrate`, otherwise it adds a new, empty column for the renamed field.

SQLite can rename and drop columns with `ALTER TABLE`, but it cannot change the type or the nullability of a column. For these changes, the table has to be created again with the new definition, and the rows copied into it. See [Making other kinds of table schema changes](https://www.sqlite.org/lang_altertable.html#otheralter) in the SQLite documentation.

SQLite cannot add a `UNIQUE` constraint to an existing column either. To make a column unique, create a unique index named the way `Migrate` names it, so that it is reported when the field is no longer unique:

```sql
CREATE UNIQUE INDEX uniq_customer_code ON customer (code);
```

## Things to keep in mind

* **Structs with column aliases** read from several tables, see [Column Aliases](../struct.md#column-aliases). `Migrate` returns an error that matches `gocrud.ErrAliasedColumnsMigrate` for them, as it would add the columns of the other tables to the table of the struct.
* **Projections**, e.g. `User_Summary` that uses the table of `User` with fewer fields, should not be migrated: all the other columns would be reported as not being fields of the struct. Migrate the full struct instead.
* **Indexes** that are reported when they are no longer declared are the ones that gocrud creates: `idx_<table>_<name>` for `idx` and `uidx`, and `uniq_<table>_<column>` described below. Indexes that you created yourself, and the ones of primary keys and `UNIQUE` constraints, are not checked for being no longer declared. A column counts as unique when it has a unique index or constraint of its own: a unique index on several columns, or with a `WHERE` condition, does not make the column unique. Drop an index that is no longer needed with `DROP INDEX "idx_customer_points"`.
* **A unique field** added in SQLite gets a unique index named `uniq_<table>_<column>` instead of a `UNIQUE` constraint, because SQLite cannot add a column with a constraint. Both reject duplicates in the same way.
* **Custom queries.** When the struct builds its own `CREATE TABLE` with `CreateTableQuery`, `Migrate` uses it to create a missing table, but compares and adds columns based on the struct fields.
* **Types of `type:` options** are compared in PostgreSQL when they are types that gocrud knows: the types of its columns, `TEXT`, `VARCHAR(n)`, `CHAR(n)`, `NUMERIC` and `DECIMAL`, whose precision and scale are not compared, `BYTEA`, `JSON`, `JSONB` and `DATE`. Other types, e.g. `type:inet`, are not compared. SQLite compares the type names as they are written.

## Registry

The [registry](../pkgs/registry.md) has a `Migrate` method that migrates the tables of all the structs in the registry, in a single transaction, and logs the differences as warnings. Like `CreateTables`, it skips projections.

```go
migrations, err := reg.Migrate(ctx)
```

`PlanMigrations` returns the migrations that `Migrate` would make, in the same order, without changing anything, and `CheckSchema` checks the tables of all the structs:

```go
plans, err := reg.PlanMigrations(ctx)

err = reg.CheckSchema(ctx)
```
