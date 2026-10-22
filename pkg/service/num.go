package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

func (c *CRUD) Num(ctx context.Context, key string, filters Filters) (uint64, error) {
	logAttrService := logger.AttrService(c, "Num")

	constructor, ok := c.registry[key]
	if !ok {
		slog.Error("key not found", logAttrService)
		return 0, ErrInvalidKey
	}

	filter, err := buildFilter(filters, logAttrService)
	if err != nil {
		return 0, err
	}

	numObjs, err := c.crud.GetCount(ctx, constructor(), gocrud.GetCountOptions{
		Filters:                  filter,
		ConvertFiltersFromString: true,
	})

	if err != nil {
		var crudErr *gocrud.CRUDError
		if errors.As(err, &crudErr) {
			var validationErr *gocrud.ValidationError
			if errors.As(crudErr.Err, &validationErr) {
				slog.Error("error with validation", logAttrService, logger.AttrError(validationErr))
				return 0, &FilterValidationError{
					Err:        validationErr,
					Violations: validationErr.Violations,
				}
			}
		}

		slog.Error("error getting", logAttrService, logger.AttrError(err))
		return 0, err
	}

	return numObjs, nil
}
