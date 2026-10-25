package registry

import (
	"context"
	"log/slog"

	"github.com/mikogs/gocrud"
)

// Read loads the record of key with the ID: a non-negative integer or a decimal string for an ID of type uint64, and a
// UUID for a string ID, so that the ID from a URL can be passed as it is. An ID of the wrong type returns an error that
// matches ErrInvalidID. A non-nil constructor loads the record into another struct, e.g. a projection.
func (c *CRUD) Read(ctx context.Context, key string, id any, constructor func() any) (any, error) {
	if constructor == nil {
		var ok bool
		constructor, ok = c.registry[key]
		if !ok {
			c.logError(ctx, "Read", "key not found", ErrInvalidKey, slog.String("key", key))
			return nil, ErrInvalidKey
		}
	}

	obj := constructor()
	if err := c.crud.Load(ctx, obj, id, gocrud.LoadOptions{}); err != nil {
		c.logError(ctx, "Read", "error reading", err, slog.Any("id", id))
		return nil, err
	}

	if gocrud.ObjIDIsZero(obj) {
		c.logError(ctx, "Read", "not found", ErrNotFound, slog.Any("id", id))
		return nil, ErrNotFound
	}

	return obj, nil
}
