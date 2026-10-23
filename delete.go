package gocrud

import "context"

type DeleteOptions struct{}

// Delete removes an object from the database table only when ID field is set.
// Once deleted from the DB, all field values are zeroed
func (c *CRUD) Delete(ctx context.Context, obj interface{}, options DeleteOptions) error {
	builder, err := c.builder(obj)
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}

	if ObjIDIsZero(obj) {
		return nil
	}

	id, err := idValue(builder.HasUUID(), ObjIDValue(obj))
	if err != nil {
		return getCRUDFuncCRUDError("delete", err)
	}

	var query string
	// if the object has a DeleteByID method, use it.
	if deleteByIDerImpl, ok := obj.(deleteByIDQueryBuilder); ok {
		query, err = deleteByIDerImpl.DeleteByIDQuery()
		if err != nil {
			return getObjFuncCRUDError("delete by id query", err)
		}
	} else {
		query = builder.DeleteByID()
	}

	_, err = c.conn.ExecContext(ctx, query, id)
	if err != nil {
		return getDBFuncCRUDError("exec", err)
	}
	zeroObjFields(obj)

	return nil
}
