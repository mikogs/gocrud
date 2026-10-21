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
