package registry

import (
	"context"
	"errors"

	"github.com/mikogs/gocrud"
)

// VerifyPasswords returns the record of key that matches the filters, e.g. the user with the e-mail of a login, and true
// when it is the only one, and all the passwords match the hashes of their fields, by field name, see
// gocrud.CRUD.VerifyPasswords. Otherwise, it returns nil and false.
func (c *CRUD) VerifyPasswords(ctx context.Context, key string, filters Filters, passwords map[string]string) (any, bool, error) {
	constructor, ok := c.registry[key]
	if !ok {
		c.logError(ctx, "VerifyPasswords", "key not found", ErrInvalidKey)
		return nil, false, ErrInvalidKey
	}

	filter, err := buildFilter(filters)
	if err != nil {
		c.logError(ctx, "VerifyPasswords", "invalid filters", err)
		return nil, false, err
	}

	obj := constructor()
	verified, err := c.crud.VerifyPasswords(ctx, obj, gocrud.VerifyPasswordsOptions{
		Filters:                  filter,
		ConvertFiltersFromString: true,
		Passwords:                passwords,
	})
	if err != nil {
		var validationErr *gocrud.ValidationError
		if errors.As(err, &validationErr) {
			err = &FilterValidationError{Err: validationErr, Violations: validationErr.Violations}
		}
		c.logError(ctx, "VerifyPasswords", "error verifying passwords", err)
		return nil, false, err
	}
	if !verified {
		return nil, false, nil
	}
	return obj, true, nil
}
