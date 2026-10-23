package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

// List returns a paginated, filtered list of records. When constructor is nil
// the registry's constructor for key is used; pass a non-nil value to scan rows
// into a different struct type (e.g. a list-specific projection).
// passFieldsToVerify maps a struct field name to a plaintext password to
// verify against the stored hash; verification only happens when the query
// matches exactly one record, see gocrud.PassOK / gocrud.PassInvalid.
func (c *CRUD) List(ctx context.Context, key string, limit, offset int, order, orderDirection string, filters Filters, rowFunc func(interface{}) interface{}, constructor func() interface{}, passFieldsToVerify map[string]string) ([]interface{}, map[string]int, error) {
	logAttrService := logger.AttrService(c, "List")

	if constructor == nil {
		var ok bool
		constructor, ok = c.registry[key]
		if !ok {
			slog.Error("key not found", logAttrService)
			return nil, map[string]int{}, ErrInvalidKey
		}
	}

	filter, err := buildFilter(filters, logAttrService)
	if err != nil {
		return nil, map[string]int{}, err
	}

	getOrder := []string{}
	if order != "" {
		getOrder = append(getOrder, order, orderDirection)
	}

	objs, passwordFields, err := c.crud.Get(ctx, constructor, gocrud.GetOptions{
		Limit:                    limit,
		Offset:                   offset,
		Order:                    getOrder,
		Filters:                  filter,
		ConvertFiltersFromString: true,
		RowObjTransformFunc:      rowFunc,
		VerifyPasswordFields:     passFieldsToVerify,
	})

	if err != nil {
		// the order usually comes from a request, like the filters
		if errors.Is(err, gocrud.ErrInvalidOrder) {
			slog.Error("invalid order", logAttrService, logger.AttrError(err))
			return nil, map[string]int{}, &FilterValidationError{
				Err:        err,
				Violations: map[string]string{order: gocrud.ViolationType},
			}
		}

		var crudErr *gocrud.CRUDError
		if errors.As(err, &crudErr) {
			var validationErr *gocrud.ValidationError
			if errors.As(crudErr.Err, &validationErr) {
				slog.Error("error with validation", logAttrService, logger.AttrError(validationErr))
				return nil, map[string]int{}, &FilterValidationError{
					Err:        validationErr,
					Violations: validationErr.Violations,
				}
			}
		}

		slog.Error("error getting", logAttrService, logger.AttrError(err))
		return nil, map[string]int{}, err
	}

	return objs, passwordFields, nil
}
