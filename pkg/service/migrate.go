package service

import (
	"context"
	"log/slog"
	"reflect"
	"sort"
	"strings"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

// Migrate runs gocrud.CRUD.Migrate for every constructor in the registry, in a single transaction, skipping
// the structs that are projections of other structs, as CreateTables does. The migrations are returned in
// the order of the registry keys.
func (c *CRUD) Migrate(ctx context.Context) ([]*gocrud.Migration, error) {
	logAttrService := logger.AttrService(c, "Migrate")

	keys := make([]string, 0, len(c.registry))
	for key := range c.registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var migrations []*gocrud.Migration
	err := c.crud.RunInTx(ctx, func(tx *gocrud.CRUD) error {
		for _, key := range keys {
			obj := c.registry[key]()

			typ := reflect.TypeOf(obj)
			for typ.Kind() == reflect.Pointer {
				typ = typ.Elem()
			}
			if strings.Contains(typ.Name(), "_") {
				continue
			}

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
