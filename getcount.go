package gocrud

import (
	"context"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

type GetCountOptions struct {
	Filters                  sqlfilters.Filter
	ConvertFiltersFromString bool
}

// GetCount runs a 'SELECT COUNT(*)' query on the database with specified filters, order, limit and offset and returns count of rows
func (c *CRUD) GetCount(ctx context.Context, obj interface{}, options GetCountOptions) (uint64, error) {
	builder, err := c.builder(obj)
	if err != nil {
		return 0, getBuilderObjectCRUDError(err)
	}

	// Filter values can be passed as string. We do not want any use of reflect outside of CRUD.
	if options.ConvertFiltersFromString {
		options.Filters, err = convertFiltersFromString(obj, options.Filters)
		if err != nil {
			return 0, err
		}
	}

	err = ValidateFilters(obj, options.Filters, c.tagName)
	if err != nil {
		return 0, err
	}

	var query string
	// if the object has a SelectCount method, use it.
	if selectCounterImpl, ok := obj.(selectCountQueryBuilder); ok {
		query, err = selectCounterImpl.SelectCountQuery(options.Filters)
		if err != nil {
			return 0, getObjFuncCRUDError("select count query", err)
		}
	} else {
		query, err = builder.SelectCount(options.Filters)
		if err != nil {
			return 0, getBuilderFuncCRUDError("select count", err)
		}
	}

	row := c.conn.QueryRowContext(ctx, query, c.args(sqlfilters.FiltersInterfaces(options.Filters))...)
	var cnt uint64
	err = row.Scan(&cnt)
	if err != nil {
		return 0, getDBFuncCRUDError("query row scan", err)
	}

	return cnt, nil
}
