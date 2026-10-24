package gocrud

import "context"

type DeleteOptions struct{}

// Delete deletes the object with the ID of obj, and sets the fields of obj to their zero values. An object without an
// ID is not deleted.
func (c *CRUD) Delete(ctx context.Context, obj interface{}, options DeleteOptions) error {
	builder, err := c.builder(obj)
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}
	ctx = c.withQuery(ctx, "Delete", builder.TableName())

	if ObjIDIsZero(obj) {
		return nil
	}

	id, err := idValue(builder.HasUUID(), ObjIDValue(obj))
	if err != nil {
		return getCRUDFuncCRUDError("delete", err)
	}

	var query string
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
