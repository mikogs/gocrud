package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

func (c *CRUD) Save(ctx context.Context, obj interface{}, now int64, userID uint64) error {
	logAttrService := logger.AttrService(c, "Save")

	err := c.crud.Save(ctx, obj, gocrud.SaveOptions{
		ModifiedAt: now,
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
