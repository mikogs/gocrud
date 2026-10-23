package registry

import (
	"context"
	"log/slog"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

// Read loads a single record by id: a non-negative integer or a decimal string for an ID of type uint64, and a UUID
// for a string ID, so that the ID from a URL can be passed as it is. An ID of the wrong type returns an error that
// matches ErrInvalidID. When constructor is nil the registry's
// constructor for key is used; pass a non-nil value to load into a
// different struct type (e.g. a read-specific projection).
// passFieldsToVerify maps a struct field name to a plaintext password to
// verify against the stored hash; the result for each password field is
// returned in the map[string]int, see gocrud.PassOK / gocrud.PassInvalid.
func (c *CRUD) Read(ctx context.Context, key string, id interface{}, constructor func() interface{}, passFieldsToVerify map[string]string) (interface{}, map[string]int, error) {
	logAttrService := logger.AttrService(c, "Read")

	if constructor == nil {
		var ok bool
		constructor, ok = c.registry[key]
		if !ok {
			slog.Error("key not found", logAttrService)
			return nil, nil, ErrInvalidKey
		}
	}

	obj := constructor()
	passwordFields, err := c.crud.Load(ctx, obj, id, gocrud.LoadOptions{
		VerifyPasswordFields: passFieldsToVerify,
	})
	if err != nil {
		slog.Error("error reading", logAttrService, slog.Any("id", id), logger.AttrError(err))
		return nil, nil, err
	}

	if gocrud.ObjIDIsZero(obj) {
		slog.Error("error not found", logAttrService, slog.Any("id", id))
		return nil, nil, ErrNotFound
	}

	return obj, passwordFields, nil
}
