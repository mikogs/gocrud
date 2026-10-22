# Versioned migrations

[`Migrate`](migrations.md) changes the tables when the application starts. It is simple, and fine for prototypes and small applications, but it only adds tables, columns and indexes, and the changes are not reviewed before they run.

A migration tool, such as [goose](https://github.com/pressly/goose), keeps the changes in files instead, e.g. `20240501103000_add_points.sql`. The files are reviewed like code, applied once and in the same order to every database, and the tool records which ones are applied. They can also make the changes that `Migrate` does not make: renaming and dropping columns, changing types, and moving data.

gocrud does not replace such a tool, but works with it:

* `PlanMigrations` writes the SQL of a new migration file for you, see [Planning a migration](migrations.md#planning-a-migration).
* `CheckSchema` checks that the migration files match the structs, in a test or when the application starts, see [Checking the schema](migrations.md#checking-the-schema).

| | `Migrate` at the start | Migration files |
|---|---|---|
| Good for | prototypes, small applications, tests | production, teams |
| Changes | tables, columns and indexes are added | anything, written or reviewed by you |
| Review | none | in the pull request with the code |
| Undo | not possible | the `Down` part of each file |

## The workflow

1. Change a struct, e.g. add a field.
2. Generate a migration file from the plan of the changes.
3. Review the file: complete it where `-- TODO:` comments show changes that gocrud does not make, and write the `Down` part.
4. Commit the file with the change of the struct. A test with `CheckSchema` fails when a file is missing.
5. The application applies the new files when it starts, or a separate command does it before the deployment.

The example below uses goose with SQLite. With PostgreSQL, use `goose.DialectPostgres` and `gocrud.DialectPostgres`. The statements are specific to the database, so generate the files with the same kind of database as the one where they are applied.

## Setting up

The structs and the service are in a package of the application, with the migration files embedded in it:

```
notes/
├── schema.go
├── migrate.go
├── schema_test.go
├── migrations/
│   └── .keep
└── cmd/
    ├── migration/main.go
    └── server/main.go
```

```go
// schema.go
package notes

import (
    "database/sql"
    "embed"
    "time"

    "github.com/mikogs/gocrud"
    svccrud "github.com/mikogs/gocrud/pkg/service"
)

// Migrations contains the migration files, which are applied with goose. "all:" embeds the directory also when it
// has only the .keep file, before the first migration is written.
//
//go:embed all:migrations
var Migrations embed.FS

type User struct {
    ID    uint64 `json:"id"`
    Email string `json:"email" validate:"required,email" crud:"uniq"`
    Name  string `json:"name" validate:"required"`
}

type Note struct {
    ID         string    `json:"id"`
    UserID     uint64    `json:"user_id" crud:"idx fk:user on_delete:cascade"`
    Title      string    `json:"title" validate:"required"`
    CreatedAt  time.Time `json:"created_at"`
    CreatedBy  uint64    `json:"created_by"`
    ModifiedAt time.Time `json:"modified_at"`
    ModifiedBy uint64    `json:"modified_by"`
}

func NewService(db *sql.DB) *svccrud.CRUD {
    return svccrud.New(map[string]func() interface{}{
        "users": func() interface{} { return &User{} },
        "notes": func() interface{} { return &Note{} },
    }, db, gocrud.DialectSQLite)
}
```

`Migrate` applies the files that are not applied yet, with goose:

```go
// migrate.go
package notes

import (
    "context"
    "database/sql"
    "errors"
    "io/fs"

    "github.com/pressly/goose/v3"
)

// Migrate applies the migration files that were not applied yet.
func Migrate(ctx context.Context, db *sql.DB) error {
    files, err := fs.Sub(Migrations, "migrations")
    if err != nil {
        return err
    }

    provider, err := goose.NewProvider(goose.DialectSQLite3, db, files)
    if errors.Is(err, goose.ErrNoMigrations) {
        return nil
    }
    if err != nil {
        return err
    }

    _, err = provider.Up(ctx)
    return err
}
```

## Generating a migration file

A small command applies the existing files to a development database, plans the changes that the structs need, and writes them to a new file. [`MigrationSQL`](migrations.md#planning-a-migration) formats the plans as SQL, with the differences that need your decision as `-- TODO:` comments:

```go
// cmd/migration/main.go

// Command migration writes a migration file with the changes that the structs need, e.g.
//
//	go run ./cmd/migration add_points
package main

import (
    "context"
    "database/sql"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "time"

    "github.com/mikogs/gocrud"
    _ "modernc.org/sqlite"

    notes "example.com/notes"
)

func main() {
    if len(os.Args) != 2 {
        log.Fatal("usage: migration <name>")
    }
    ctx := context.Background()

    // the development database, with all the migration files applied
    db, err := sql.Open("sqlite", "file:dev.db?_pragma=foreign_keys(1)")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    if err := notes.Migrate(ctx, db); err != nil {
        log.Fatalf("applying the migrations: %v", err)
    }

    plans, err := notes.NewService(db).PlanMigrations(ctx)
    if err != nil {
        log.Fatal(err)
    }

    up := gocrud.MigrationSQL(plans...)
    if up == "" {
        fmt.Println("the database matches the structs, there is nothing to migrate")
        return
    }

    name := filepath.Join("migrations", time.Now().UTC().Format("20060102150405")+"_"+os.Args[1]+".sql")
    content := "-- +goose Up\n" + up + "\n-- +goose Down\n-- TODO: undo the changes above\n"
    if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
        log.Fatal(err)
    }
    fmt.Println("written", name)
}
```

The first run writes the file that creates the tables:

```
$ go run ./cmd/migration init
written migrations/20240501100000_init.sql
```

```sql
-- +goose Up
-- table "user"
CREATE TABLE IF NOT EXISTS "user" ("id" INTEGER PRIMARY KEY,"email" TEXT NOT NULL DEFAULT '' UNIQUE,"name" TEXT NOT NULL DEFAULT '');

-- table "note"
CREATE TABLE IF NOT EXISTS "note" ("id" TEXT PRIMARY KEY,"user_id" INTEGER NOT NULL DEFAULT 0 REFERENCES "user"("id") ON DELETE CASCADE,"title" TEXT NOT NULL DEFAULT '',"created_at" DATETIME NOT NULL DEFAULT '0001-01-01 00:00:00.000000000+00:00',"created_by" INTEGER NOT NULL DEFAULT 0,"modified_at" DATETIME NOT NULL DEFAULT '0001-01-01 00:00:00.000000000+00:00',"modified_by" INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS "idx_note_user_id" ON "note" ("user_id");

-- +goose Down
-- TODO: undo the changes above
```

Write the `Down` part, so that the migration can be undone, e.g. with `goose down`:

```sql
-- +goose Down
DROP TABLE "note";
DROP TABLE "user";
```

## Changing a struct

Add fields to `User`:

```go
type User struct {
    ID     uint64  `json:"id"`
    Email  string  `json:"email" validate:"required,email" crud:"uniq"`
    Name   string  `json:"name" validate:"required"`
    Points int64   `json:"points" crud:"idx"`
    Bio    *string `json:"bio"`
}
```

```
$ go run ./cmd/migration add_points
written migrations/20240502090000_add_points.sql
```

```sql
-- +goose Up
-- table "user"
ALTER TABLE "user" ADD COLUMN "points" INTEGER NOT NULL DEFAULT 0;
ALTER TABLE "user" ADD COLUMN "bio" TEXT;
CREATE INDEX IF NOT EXISTS "idx_user_points" ON "user" ("points");

-- +goose Down
DROP INDEX "idx_user_points";
ALTER TABLE "user" DROP COLUMN "bio";
ALTER TABLE "user" DROP COLUMN "points";
```

## Changes that you write

gocrud plans only the changes that cannot lose data, and lists the other differences as `-- TODO:` comments. Always read a generated file, because a change that looks simple in Go may need a different change in the database. For example, when `Note.Title` is renamed to `Subject`, the generated file adds a new, empty column, and the titles stay in the old one:

```sql
-- +goose Up
-- table "note"
ALTER TABLE "note" ADD COLUMN "subject" TEXT NOT NULL DEFAULT '';
-- TODO: column "title" is not a field of the struct
```

Replace it with a rename, which keeps the data:

```sql
-- +goose Up
ALTER TABLE "note" RENAME COLUMN "title" TO "subject";

-- +goose Down
ALTER TABLE "note" RENAME COLUMN "subject" TO "title";
```

The same goes for the other [changes that are not made](migrations.md#changes-that-are-not-made), e.g. changing the type of a column, making a column `NOT NULL` after filling it, or converting numeric IDs to UUIDs. Write them as SQL, and run `CheckSchema` to check the result.

## Checking the files in a test

The test applies all the files to an empty database, and checks that the tables match the structs. It fails when a struct was changed without a migration file, or when a file does not do what the struct needs, e.g. the generated file of the rename above, which leaves the `title` column:

```go
// schema_test.go
package notes

import (
    "context"
    "database/sql"
    "path/filepath"
    "testing"

    _ "modernc.org/sqlite"
)

func TestMigrationsMatchStructs(t *testing.T) {
    ctx := context.Background()

    db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "test.db")+"?_pragma=foreign_keys(1)")
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    if err := Migrate(ctx, db); err != nil {
        t.Fatalf("applying the migrations: %v", err)
    }
    if err := NewService(db).CheckSchema(ctx); err != nil {
        t.Fatal(err)
    }
}
```

```
--- FAIL: TestMigrationsMatchStructs
    schema_test.go:25: database schema differs from the structs: table "note": column "title" is not a field of the struct
```

## Applying the files

The application applies the new files when it starts, and checks the result before it serves requests:

```go
// cmd/server/main.go
func main() {
    ctx := context.Background()

    db, err := sql.Open("sqlite", "file:prod.db?_pragma=foreign_keys(1)")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    if err := notes.Migrate(ctx, db); err != nil {
        log.Fatalf("applying the migrations: %v", err)
    }

    svc := notes.NewService(db)
    if err := svc.CheckSchema(ctx); err != nil {
        log.Fatalf("the migrations do not match the structs: %v", err)
    }

    // serve requests with svc
}
```

When several instances of the application start at the same time, apply the files from one place only, e.g. with the [goose command](https://github.com/pressly/goose#usage) before the deployment, and keep only `CheckSchema` in the application.

## Other tools

The generated SQL works with any tool that runs SQL files:

* **[golang-migrate](https://github.com/golang-migrate/migrate)** keeps each migration in two files, e.g. `000002_add_points.up.sql` and `000002_add_points.down.sql`. Write the output of `MigrationSQL` to the `up` file.
* **[Atlas](https://atlasgo.io)** in the versioned mode applies the SQL files of a directory, which it checks with an `atlas.sum` file. Write the output of `MigrationSQL` to a new file, and update the sum with `atlas migrate hash`.
