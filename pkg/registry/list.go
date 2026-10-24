package registry

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mikogs/gocrud"
)

// ListOptions are the options of List.
type ListOptions struct {
	Limit  int
	Offset int
	// Order is the name of a field, or of a path through relation fields, and OrderDirection is "asc" or "desc".
	Order          string
	OrderDirection string
	// After is the cursor of NextCursor: List returns the records after the record of the cursor, in the same order.
	// It cannot be combined with an Offset.
	After   string
	Filters Filters
	// RowFunc is called with each record, and List returns its results, see gocrud.GetOptions.RowObjTransformFunc.
	RowFunc func(any) any
	// Constructor scans the rows into another struct than the one of the key, e.g. a projection.
	Constructor func() any
}

// List returns a page of the records of key that match the options.
func (c *CRUD) List(ctx context.Context, key string, options ListOptions) ([]any, error) {
	constructor := options.Constructor
	if constructor == nil {
		var ok bool
		constructor, ok = c.registry[key]
		if !ok {
			c.logError(ctx, "List", "key not found", ErrInvalidKey, slog.String("key", key))
			return nil, ErrInvalidKey
		}
	}

	filter, err := buildFilter(options.Filters)
	if err != nil {
		c.logError(ctx, "List", "invalid filters", err)
		return nil, err
	}

	objs, err := c.crud.Get(ctx, constructor, gocrud.GetOptions{
		Limit:                    options.Limit,
		Offset:                   options.Offset,
		Order:                    listOrder(options.Order, options.OrderDirection),
		After:                    options.After,
		Filters:                  filter,
		ConvertFiltersFromString: true,
		RowObjTransformFunc:      options.RowFunc,
	})

	if err != nil {
		// the order and the cursor usually come from a request, like the filters
		if errors.Is(err, gocrud.ErrInvalidOrder) {
			err = &FilterValidationError{
				Err:        err,
				Violations: map[string]string{options.Order: gocrud.ViolationType},
			}
			c.logError(ctx, "List", "invalid order", err)
			return nil, err
		}
		if errors.Is(err, gocrud.ErrCursor) {
			err = &FilterValidationError{
				Err:        err,
				Violations: map[string]string{"after": gocrud.ViolationType},
			}
			c.logError(ctx, "List", "invalid order", err)
			return nil, err
		}

		var crudErr *gocrud.CRUDError
		if errors.As(err, &crudErr) {
			var validationErr *gocrud.ValidationError
			if errors.As(crudErr.Err, &validationErr) {
				err = &FilterValidationError{
					Err:        validationErr,
					Violations: validationErr.Violations,
				}
				c.logError(ctx, "List", "error with validation", err)
				return nil, err
			}
		}

		c.logError(ctx, "List", "error getting", err)
		return nil, err
	}

	return objs, nil
}

// NextCursor returns the cursor of the page after obj, the last record of a page of List with the order, for
// ListOptions.After, see gocrud.CRUD.NextCursor.
func (c *CRUD) NextCursor(obj any, order string, orderDirection string) (string, error) {
	return c.crud.NextCursor(obj, listOrder(order, orderDirection))
}

func listOrder(order string, orderDirection string) []string {
	if order == "" {
		return nil
	}
	return []string{order, orderDirection}
}
