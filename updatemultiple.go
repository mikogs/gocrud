package gocrud

import (
	"context"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	sqlbuilder "github.com/mikogs/gocrud/pkg/sql/builder"
)

type UpdateMultipleOptions struct {
	Filters                 *sqlfilters.Filters
	ConvertValuesFromString bool
}

func (c *CRUD) UpdateMultiple(ctx context.Context, obj interface{}, fieldsToUpdate map[string]interface{}, options UpdateMultipleOptions) error {
	builder, err := c.builder(obj)
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}
	if options.ConvertValuesFromString {
		for name, value := range fieldsToUpdate {
			valueAsString, ok := value.(string)
			if !ok {
				continue
			}

			ok, valueAsFieldType := sqlbuilder.StructFieldValueFromString(obj, name, valueAsString)
			if !ok {
				return getStructFieldValueFromStringCRUDError()
			}
			fieldsToUpdate[name] = valueAsFieldType
		}
	}
	ok, violations, err := ValidateValues(obj, fieldsToUpdate, c.tagName)
	if err != nil {
		return getValidateObjCRUDError(err)
	}

	if !ok {
		return getObjInvalidCRUDError(violations)
	}

	err = ValidateFilters(obj, options.Filters, c.tagName)
	if err != nil {
		return err
	}

	var query string
	// if the object has an Update method, use it.
	if updateerImpl, ok := obj.(updateQueryBuilder); ok {
		query, err = updateerImpl.UpdateQuery(fieldsToUpdate, options.Filters)
		if err != nil {
			return getObjFuncCRUDError("update query", err)
		}
	} else {
		query, err = builder.Update(fieldsToUpdate, options.Filters)
		if err != nil {
			return getBuilderFuncCRUDError("update", err)
		}
	}

	_, err = c.conn.ExecContext(ctx, query, append(sqlbuilder.MapInterfaces(fieldsToUpdate), sqlfilters.FiltersInterfaces(options.Filters)...)...)
	if err != nil {
		return getDBFuncCRUDError("exec", err)
	}

	return nil
}
