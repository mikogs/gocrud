package registry

import (
	"context"
	"errors"

	"github.com/mikogs/gocrud"
)

// Save saves obj in the database. userID is stored in the ModifiedBy and CreatedBy fields, when obj has them: a
// non-negative integer for fields of type uint64, and a string for fields of type string, see gocrud.SaveOptions.
func (c *CRUD) Save(ctx context.Context, obj interface{}, userID interface{}) error {
	err := c.crud.Save(ctx, obj, gocrud.SaveOptions{
		ModifiedBy: userID,
	})
	if err != nil {
		var crudErr *gocrud.CRUDError
		if errors.As(err, &crudErr) {
			var validationErr *gocrud.ValidationError
			if errors.As(crudErr.Err, &validationErr) {
				err = &ModelValidationError{
					Err:        validationErr,
					Violations: validationErr.Violations,
				}
				c.logError(ctx, "Save", "error with validation", err)
				return err
			}
		}

		c.logError(ctx, "Save", "error saving", err)
		return err
	}

	return nil
}
