package registry

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mikogs/gocrud"
)

func (c *CRUD) Num(ctx context.Context, key string, filters Filters) (uint64, error) {
	constructor, ok := c.registry[key]
	if !ok {
		c.logError(ctx, "Num", "key not found", ErrInvalidKey, slog.String("key", key))
		return 0, ErrInvalidKey
	}

	filter, err := buildFilter(filters)
	if err != nil {
		c.logError(ctx, "Num", "invalid filters", err)
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
				err = &FilterValidationError{
					Err:        validationErr,
					Violations: validationErr.Violations,
				}
				c.logError(ctx, "Num", "error with validation", err)
				return 0, err
			}
		}

		c.logError(ctx, "Num", "error getting", err)
		return 0, err
	}

	return numObjs, nil
}
