package registry

import (
	"context"
	"log/slog"

	"github.com/mikogs/gocrud"
)

// Migrate runs gocrud.CRUD.Migrate for every constructor in the registry, in a single transaction, skipping
// the structs that are projections of other structs, as CreateTables does. The tables are migrated, and the
// migrations returned, in the order of the registry keys, except that the tables referenced by foreign keys
// come before the tables that reference them.
func (c *CRUD) Migrate(ctx context.Context) ([]*gocrud.Migration, error) {
	keys, err := c.tableKeys()
	if err != nil {
		c.logError(ctx, "Migrate", "error ordering tables", err)
		return nil, err
	}

	var migrations []*gocrud.Migration
	err = c.crud.RunInTx(ctx, func(tx *gocrud.CRUD) error {
		for _, key := range keys {
			obj := c.registry[key]()

			migration, err := tx.Migrate(ctx, obj)
			if err != nil {
				c.logError(ctx, "Migrate", "error migrating table", err, slog.String("key", key))
				return err
			}

			for _, difference := range migration.Differences {
				c.log().LogAttrs(ctx, slog.LevelWarn, "table differs from struct", methodAttr("Migrate"),
					slog.String("table", migration.Table), slog.String("difference", difference))
			}
			migrations = append(migrations, migration)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return migrations, nil
}

// PlanMigrations returns the changes that Migrate would make to the tables of the structs in the registry, without
// making them, in the order in which Migrate would make them. A table that references a table that does not exist
// yet is planned as it would be after the referenced table is created.
func (c *CRUD) PlanMigrations(ctx context.Context) ([]*gocrud.Migration, error) {
	keys, err := c.tableKeys()
	if err != nil {
		c.logError(ctx, "PlanMigrations", "error ordering tables", err)
		return nil, err
	}

	migrations := make([]*gocrud.Migration, 0, len(keys))
	for _, key := range keys {
		migration, err := c.crud.PlanMigration(ctx, c.registry[key]())
		if err != nil {
			c.logError(ctx, "PlanMigrations", "error planning migration", err, slog.String("key", key))
			return nil, err
		}
		migrations = append(migrations, migration)
	}

	return migrations, nil
}

// CheckSchema checks that the tables of the structs in the registry match the structs, see gocrud.CRUD.CheckSchema.
// It returns a *gocrud.SchemaError, which matches gocrud.ErrSchemaDiffers, when they do not.
func (c *CRUD) CheckSchema(ctx context.Context) error {
	keys, err := c.tableKeys()
	if err != nil {
		return err
	}

	objs := make([]any, 0, len(keys))
	for _, key := range keys {
		objs = append(objs, c.registry[key]())
	}

	return c.crud.CheckSchema(ctx, objs...)
}
