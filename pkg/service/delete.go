package service

import (
	"context"
	"log/slog"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

// Delete deletes the record with the id, of the types that Read accepts.
func (c *CRUD) Delete(ctx context.Context, key string, id interface{}) error {
	logAttrService := logger.AttrService(c, "Delete")

	obj, _, err := c.Read(ctx, key, id, nil, nil)
	if err != nil {
		return err
	}

	err = c.crud.Delete(ctx, obj, gocrud.DeleteOptions{})
	if err != nil {
		slog.Error("error deleting", logAttrService, slog.Any("id", id))
		return err
	}

	return nil
}
