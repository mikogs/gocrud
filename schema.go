package gocrud

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrSchemaDiffers is matched by the error of CheckSchema when the tables do not match the structs.
var ErrSchemaDiffers = errors.New("database schema differs from the structs")

// SchemaError is returned by CheckSchema. It describes the tables that do not match the structs, with the changes
// that Migrate would make, and the differences that it would report.
type SchemaError struct {
	Migrations []*Migration
}

func (e *SchemaError) Error() string {
	var details []string
	for _, migration := range e.Migrations {
		for _, statement := range migration.Statements {
			details = append(details, fmt.Sprintf(`table "%s": %s`, migration.Table, statement))
		}
		for _, difference := range migration.Differences {
			details = append(details, fmt.Sprintf(`table "%s": %s`, migration.Table, difference))
		}
	}

	return ErrSchemaDiffers.Error() + ": " + strings.Join(details, "; ")
}

func (e *SchemaError) Unwrap() error {
	return ErrSchemaDiffers
}

// CheckSchema checks that the tables of objs match the structs: that Migrate would not change them, and would not
// report differences. Otherwise, it returns a *SchemaError, which matches ErrSchemaDiffers. Use it in a test, or at
// the start of an application, when the tables are changed by a migration tool instead of Migrate.
func (c *CRUD) CheckSchema(ctx context.Context, objs ...any) error {
	migrations := make([]*Migration, 0, len(objs))
	for _, obj := range objs {
		migration, err := c.PlanMigration(ctx, obj)
		if err != nil {
			return err
		}
		migrations = append(migrations, migration)
	}

	return newSchemaError(migrations)
}

// newSchemaError returns a *SchemaError for the migrations that have statements or differences, or nil when there
// are none.
func newSchemaError(migrations []*Migration) error {
	var differing []*Migration
	for _, migration := range migrations {
		if len(migration.Statements) > 0 || len(migration.Differences) > 0 {
			differing = append(differing, migration)
		}
	}

	if len(differing) == 0 {
		return nil
	}
	return &SchemaError{Migrations: differing}
}
