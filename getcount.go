package gocrud

import (
	"context"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type GetCountOptions struct {
	Filters                  sqlfilters.Filter
	ConvertFiltersFromString bool
}

// GetCount returns the number of the rows that match the filters.
func (c *CRUD) GetCount(ctx context.Context, obj interface{}, options GetCountOptions) (uint64, error) {
	builder, err := c.builder(obj)
	if err != nil {
		return 0, getBuilderObjectCRUDError(err)
	}
	ctx = c.withQuery(ctx, "GetCount", builder.TableName())

	options.Filters, err = c.prepareFilters(obj, options.Filters, options.ConvertFiltersFromString)
	if err != nil {
		return 0, err
	}

	var query string
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
