package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

// Read loads a single record by id. When constructor is nil the registry's
// constructor for key is used; pass a non-nil value to load into a
// different struct type (e.g. a read-specific projection).
// passFieldsToVerify maps a struct field name to a plaintext password to
// verify against the stored hash; the result for each password field is
// returned in the map[string]int, see gocrud.PassOK / gocrud.PassInvalid.
func (c *CRUD) Read(ctx context.Context, key string, id uint64, constructor func() interface{}, passFieldsToVerify map[string]string) (interface{}, map[string]int, error) {
	logAttrService := logger.AttrService(c, "Read")

	if constructor == nil {
		var ok bool
		constructor, ok = c.registry[key]
		if !ok {
			slog.Error("key not found", logAttrService)
			return nil, nil, InvalidKeyError
		}
	}

	obj := constructor()
	output := c.crud.Load(ctx, obj, fmt.Sprintf("%d", id), gocrud.LoadOptions{
		VerifyPasswordFields: passFieldsToVerify,
	})
	if output.Error != nil {
		return nil, nil, output.Error
	}

	objID := gocrud.ObjIDValue(obj)
	if objID == 0 {
		slog.Error("error not found", logAttrService, slog.Uint64("id", id))
		return nil, nil, NotFoundError
	}

	return obj, output.PasswordFields, nil
}
