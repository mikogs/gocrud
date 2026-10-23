package registry

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

// Save saves obj in the database. userID is stored in the ModifiedBy and CreatedBy fields, when obj has them: a
// non-negative integer for fields of type uint64, and a string for fields of type string, see gocrud.SaveOptions.
func (c *CRUD) Save(ctx context.Context, obj interface{}, userID interface{}) error {
	logAttrService := logger.AttrService(c, "Save")

	err := c.crud.Save(ctx, obj, gocrud.SaveOptions{
		ModifiedBy: userID,
	})
	if err != nil {
		var crudErr *gocrud.CRUDError
		if errors.As(err, &crudErr) {
			var validationErr *gocrud.ValidationError
			if errors.As(crudErr.Err, &validationErr) {
				slog.Error("error with validation", logAttrService, logger.AttrError(err))
				return &ModelValidationError{
					Err:        validationErr,
					Violations: validationErr.Violations,
				}
			}
		}

		slog.Error("error saving", logAttrService, logger.AttrError(err))
		return err
	}

	return nil
}
