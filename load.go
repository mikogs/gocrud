package gocrud

import (
	"context"
	"database/sql"
	"errors"
)

type LoadOptions struct {
	// VerifyPasswordFields maps the names of password fields to the passwords to verify with their hashes.
	VerifyPasswordFields map[string]string

	// Preload contains the names of relation fields whose related objects are loaded, e.g. []string{"Customer", "Items"}.
	Preload []string
}

// Load sets obj to the object with the given ID: a non-negative integer or a decimal string for an ID of type uint64,
// and a UUID for a string ID. When there is no such object, all the fields of obj are set to their zero values.
// It returns PassOK or PassInvalid for every password field that is also in options.VerifyPasswordFields. Keys in
// VerifyPasswordFields that do not name an actual password field are ignored.
func (c *CRUD) Load(ctx context.Context, obj interface{}, id interface{}, options LoadOptions) (map[string]int, error) {
	passwordFields := map[string]int{}

	builder, err := c.builder(obj)
	if err != nil {
		return nil, getBuilderObjectCRUDError(err)
	}

	idArg, err := idValue(builder.HasUUID(), id)
	if err != nil {
		return nil, getCRUDFuncCRUDError("load", err)
	}

	if err := c.checkPreload(builder, options.Preload, ""); err != nil {
		return nil, getCRUDFuncCRUDError("preload", err)
	}

	var query string

	if selectByIDerImpl, ok := obj.(selectByIDQueryBuilder); ok {
		query, err = selectByIDerImpl.SelectByIDQuery()
		if err != nil {
			return nil, getObjFuncCRUDError("select by id query", err)
		}
	} else {
		query = builder.SelectByID()
	}

	err = c.conn.QueryRowContext(ctx, query, idArg).Scan(objFieldPointers(obj, builder.ColumnIndexes(), nil)...)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		zeroObjFields(obj)
		verifyWithoutObject(builder.PasswordFields(), options.VerifyPasswordFields)
		return passwordFields, nil
	case err != nil:
		return nil, getDBFuncCRUDError("query row", err)
	}

	timeFieldsToUTC(obj, builder.TimeIndexes())

	for _, fieldName := range builder.PasswordFields() {
		password, ok := options.VerifyPasswordFields[fieldName]
		if !ok {
			continue
		}

		hash, _ := objFieldValue(obj, fieldName).(string)
		passwordFields[fieldName] = verifyPassword(hash, password)
	}

	zeroPasswordFields(obj, builder.PasswordIndexes())

	if err := c.preload(ctx, builder, []interface{}{obj}, options.Preload); err != nil {
		return nil, err
	}

	return passwordFields, nil
}
