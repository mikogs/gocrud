package service

import (
	"context"
	"log/slog"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

func (c *CRUD) Delete(ctx context.Context, key string, id uint64) error {
	logAttrService := logger.AttrService(c, "Delete")

	obj, _, err := c.Read(ctx, key, id, nil, nil)
	if err != nil {
		return err
	}

	err = c.crud.Delete(ctx, obj, gocrud.DeleteOptions{})
	if err != nil {
		slog.Error("error deleting", logAttrService, slog.Uint64("id", id))
		return err
	}

	return nil
}
