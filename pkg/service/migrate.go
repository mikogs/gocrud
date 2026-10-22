package service

import (
	"context"
	"log/slog"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

// Migrate runs gocrud.CRUD.Migrate for every constructor in the registry, in a single transaction, skipping
// the structs that are projections of other structs, as CreateTables does. The tables are migrated, and the
// migrations returned, in the order of the registry keys, except that the tables referenced by foreign keys
// come before the tables that reference them.
func (c *CRUD) Migrate(ctx context.Context) ([]*gocrud.Migration, error) {
	logAttrService := logger.AttrService(c, "Migrate")

	keys, err := c.tableKeys()
	if err != nil {
		slog.Error("error ordering tables", logAttrService, logger.AttrError(err))
		return nil, err
	}

	var migrations []*gocrud.Migration
	err = c.crud.RunInTx(ctx, func(tx *gocrud.CRUD) error {
		for _, key := range keys {
			obj := c.registry[key]()

			migration, err := tx.Migrate(ctx, obj)
			if err != nil {
				slog.Error("error migrating table", logAttrService, slog.String("key", key), logger.AttrError(err))
				return err
			}

			for _, difference := range migration.Differences {
				slog.Warn("table differs from struct", logAttrService, slog.String("table", migration.Table), slog.String("difference", difference))
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
	logAttrService := logger.AttrService(c, "PlanMigrations")

	keys, err := c.tableKeys()
	if err != nil {
		slog.Error("error ordering tables", logAttrService, logger.AttrError(err))
		return nil, err
	}

	migrations := make([]*gocrud.Migration, 0, len(keys))
	for _, key := range keys {
		migration, err := c.crud.PlanMigration(ctx, c.registry[key]())
		if err != nil {
			slog.Error("error planning migration", logAttrService, slog.String("key", key), logger.AttrError(err))
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

	objs := make([]interface{}, 0, len(keys))
	for _, key := range keys {
		objs = append(objs, c.registry[key]())
	}

	return c.crud.CheckSchema(ctx, objs...)
}
