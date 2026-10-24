package gocrud

import (
	"context"
	"database/sql"
	"errors"
)

type LoadOptions struct {
	// Preload contains the names of relation fields whose related objects are loaded, e.g. []string{"Customer", "Items"}.
	Preload []string
}

// Load sets obj to the object with the given ID: a non-negative integer or a decimal string for an ID of type uint64,
// and a UUID for a string ID. When there is no such object, all the fields of obj are set to their zero values. The
// password fields of obj are empty, see VerifyPasswords.
func (c *CRUD) Load(ctx context.Context, obj any, id any, options LoadOptions) error {
	builder, err := c.builder(obj)
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}
	ctx = c.withQuery(ctx, "Load", builder.TableName())

	idArg, err := idValue(builder.HasUUID(), id)
	if err != nil {
		return getCRUDFuncCRUDError("load", err)
	}

	if err := c.checkPreload(builder, options.Preload, ""); err != nil {
		return getCRUDFuncCRUDError("preload", err)
	}

	var query string

	if selectByIDerImpl, ok := obj.(selectByIDQueryBuilder); ok {
		query, err = selectByIDerImpl.SelectByIDQuery()
		if err != nil {
			return getObjFuncCRUDError("select by id query", err)
		}
	} else {
		query = builder.SelectByID()
	}

	err = c.conn.QueryRowContext(ctx, query, idArg).Scan(objScanPointers(obj, builder.ColumnIndexes(), builder.FieldKinds(), nil)...)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		zeroObjFields(obj)
		return nil
	case err != nil:
		return getDBFuncCRUDError("query row", err)
	}

	timeFieldsToUTC(obj, builder.TimeIndexes())

	zeroPasswordFields(obj, builder.PasswordIndexes())

	return c.preload(ctx, builder, []any{obj}, options.Preload)
}
