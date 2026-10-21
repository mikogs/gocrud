package gocrud

import (
	"context"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

type DeleteMultipleOptions struct {
	Filters *sqlfilters.Filters
}

// DeleteMultiple removes objects from the database based on specified filters, and returns the IDs of the
// removed objects. Rows of other tables that are deleted by foreign keys are not included.
func (c *CRUD) DeleteMultiple(ctx context.Context, obj interface{}, options DeleteMultipleOptions) ([]uint64, error) {
	builder, err := c.builder(obj)
	if err != nil {
		return nil, getBuilderObjectCRUDError(err)
	}

	err = ValidateFilters(obj, options.Filters, c.tagName)
	if err != nil {
		return nil, err
	}

	var query string
	// if the object has a DeleteReturningID method, use it.
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
	defer rows.Close()

	deletedIDs := []uint64{}
	for rows.Next() {
		var deletedID uint64
		if err := rows.Scan(&deletedID); err != nil {
			return nil, getDBFuncCRUDError("rows scan", err)
		}

		deletedIDs = append(deletedIDs, deletedID)
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("rows", err)
	}

	return deletedIDs, nil
}
