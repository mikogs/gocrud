package gocrud

import (
	"context"
	"reflect"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type DeleteMultipleOptions struct {
	Filters sqlfilters.Filter
}

// DeleteMultiple removes objects from the database based on specified filters, and returns the IDs of the
// removed objects, of the type of the ID field. Rows of other tables that are deleted by foreign keys are not included.
func (c *CRUD) DeleteMultiple(ctx context.Context, obj any, options DeleteMultipleOptions) ([]any, error) {
	builder, err := c.builder(obj)
	if err != nil {
		return nil, getBuilderObjectCRUDError(err)
	}
	ctx = c.withQuery(ctx, "DeleteMultiple", builder.TableName())

	options.Filters, err = c.prepareFilters(obj, options.Filters, false)
	if err != nil {
		return nil, err
	}

	var query string
	if deleteReturningIDerImpl, ok := obj.(deleteReturningIDQueryBuilder); ok {
		query, err = deleteReturningIDerImpl.DeleteReturningIDQuery()
		if err != nil {
			return nil, getObjFuncCRUDError("delete returning id query", err)
		}
	} else {
		query, err = builder.DeleteReturningID(options.Filters)
		if err != nil {
			return nil, getBuilderFuncCRUDError("delete returning id", err)
		}
	}

	rows, err := c.conn.QueryContext(ctx, query, c.args(sqlfilters.FiltersInterfaces(options.Filters))...)
	if err != nil {
		return nil, getDBFuncCRUDError("query", err)
	}
	defer func() { _ = rows.Close() }()

	deletedIDs := []any{}
	for rows.Next() {
		deletedID := reflect.New(reflect.ValueOf(obj).Elem().FieldByName(IDField).Type())
		if err := rows.Scan(deletedID.Interface()); err != nil {
			return nil, getDBFuncCRUDError("rows scan", err)
		}

		deletedIDs = append(deletedIDs, deletedID.Elem().Interface())
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("rows", err)
	}

	return deletedIDs, nil
}
