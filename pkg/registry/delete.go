package registry

import (
	"context"
	"log/slog"

	"github.com/mikogs/gocrud"
)

// Delete deletes the record with the id, of the types that Read accepts.
func (c *CRUD) Delete(ctx context.Context, key string, id interface{}) error {
	obj, _, err := c.Read(ctx, key, id, nil, nil)
	if err != nil {
		return err
	}

	err = c.crud.Delete(ctx, obj, gocrud.DeleteOptions{})
	if err != nil {
		c.logError(ctx, "Delete", "error deleting", err, slog.Any("id", id))
		return err
	}

	return nil
}
