package service

import (
	"context"
	"testing"

	"github.com/mikogs/gocrud"
)

type Customer struct {
	ID    uint64
	Name  string
	Email *string `crud:"uniq"`
}

type Customer_Summary struct {
	ID   uint64
	Name string
}

func TestMigrate(t *testing.T) {
	ctx := context.Background()
	svc := New(map[string]func() interface{}{
		"customers":         func() interface{} { return &Customer{} },
		"customersummaries": func() interface{} { return &Customer_Summary{} },
	}, testDB, gocrud.DialectPostgres)

	_ = testCRUD.DropTable(ctx, &Customer{})
	if _, err := testDB.ExecContext(ctx, `CREATE TABLE customer (id SERIAL PRIMARY KEY, name VARCHAR(255) NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("creating the table failed: %s", err.Error())
	}

	migrations, err := svc.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate failed: %s", err.Error())
	}
	if len(migrations) != 1 || migrations[0].Table != "customer" || len(migrations[0].AddedColumns) != 1 ||
		migrations[0].AddedColumns[0] != "email" || len(migrations[0].Differences) != 0 {
		t.Fatalf("Migrate returned %+v, want the email column added to the customer table only", migrations)
	}

	if err := svc.Save(ctx, &Customer{Name: "Acme"}, 0); err != nil {
		t.Fatalf("Save failed in the migrated table: %s", err.Error())
	}
}
