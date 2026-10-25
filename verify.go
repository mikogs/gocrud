package gocrud

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

// ErrVerifyPasswords is matched by the error of VerifyPasswords without passwords, or with a field that is not a
// password field, which could not be verified.
var ErrVerifyPasswords = errors.New("invalid passwords to verify")

type VerifyPasswordsOptions struct {
	// Filters select the object, e.g. the user with the e-mail of a login, as in Get.
	Filters                  sqlfilters.Filter
	ConvertFiltersFromString bool
	// Passwords are the passwords to verify, by the names of the password fields.
	Passwords map[string]string
}

// VerifyPasswords sets obj to the object that matches the filters, e.g. the user with the e-mail of a login, and returns
// true when it is the only one, and all the passwords match the hashes of their fields. Otherwise, it sets the fields
// of obj to their zero values, and returns false. The passwords are compared also when no object or more than one
// matches, with a dummy hash, so that the time of VerifyPasswords does not show whether such an object exists.
func (c *CRUD) VerifyPasswords(ctx context.Context, obj any, options VerifyPasswordsOptions) (bool, error) {
	builder, err := c.builder(obj)
	if err != nil {
		return false, getBuilderObjectCRUDError(err)
	}
	// without a password, the objects would be verified only by the filters
	if len(options.Passwords) == 0 {
		return false, getCRUDFuncCRUDError("verify passwords", fmt.Errorf("%w: no passwords", ErrVerifyPasswords))
	}
	for field := range options.Passwords {
		if !slices.Contains(builder.PasswordFields(), field) {
			return false, getCRUDFuncCRUDError("verify passwords", fmt.Errorf("%w: %s is not a password field", ErrVerifyPasswords, field))
		}
	}

	objType := reflect.TypeOf(obj).Elem()
	// two objects are enough to know that the filters do not match only one
	objs, hashes, err := c.get(ctx, func() any { return reflect.New(objType).Interface() }, GetOptions{
		Filters:                  options.Filters,
		ConvertFiltersFromString: options.ConvertFiltersFromString,
		Limit:                    2,
	}, true)
	if err != nil {
		return false, err
	}

	zeroObjFields(obj)
	if len(objs) != 1 {
		verifyWithoutObject(builder.PasswordFields(), options.Passwords)
		return false, nil
	}

	verified := true
	// all the passwords are compared, so that the time does not show which of them does not match
	for field, password := range options.Passwords {
		if !verifyPassword(hashes[0][field], password) {
			verified = false
		}
	}
	if verified {
		reflect.ValueOf(obj).Elem().Set(reflect.ValueOf(objs[0]).Elem())
	}
	return verified, nil
}
