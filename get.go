package gocrud

import (
	"context"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	"golang.org/x/crypto/bcrypt"
)

type GetOptions struct {
	Order                    []string
	Limit                    int
	Offset                   int
	Filters                  *sqlfilters.Filters
	RowObjTransformFunc      func(interface{}) interface{}
	ConvertFiltersFromString bool

	// VerifyPasswordFields maps a struct field name to a plaintext password
	// to verify against the bcrypt hash stored in that field. Verification
	// only happens when the query matches exactly one record; for any other
	// result count the returned map is empty.
	VerifyPasswordFields map[string]string
}

// Get returns the matching records and, for every password field that also
// appears in options.VerifyPasswordFields, either PassOK or PassInvalid —
// but only when exactly one record was found. Keys in VerifyPasswordFields
// that do not name an actual password field are ignored.
func (c *CRUD) Get(ctx context.Context, newObjFunc func() interface{}, options GetOptions) ([]interface{}, map[string]int, error) {
	obj := newObjFunc()

	builder, err := c.builder(obj)
	if err != nil {
		return nil, nil, getBuilderObjectCRUDError(err)
	}

	// Filter values can be passed as string. We do not want any use of reflect outside of CRUD.
	if options.ConvertFiltersFromString {
		options.Filters, err = convertFiltersFromString(obj, options.Filters)
		if err != nil {
			return nil, nil, err
		}
	}

	err = ValidateFilters(obj, options.Filters, c.tagName)
	if err != nil {
		return nil, nil, err
	}

	var returnValue []interface{}
	var passwordHashesByRow []map[string]string

	var query string

	// if the object has a Select method, use it.
	if selectedImpl, ok := obj.(selectQueryBuilder); ok {
		query, err = selectedImpl.SelectQuery(options.Order, options.Limit, options.Offset, options.Filters)
		if err != nil {
			return nil, nil, getObjFuncCRUDError("select query", err)
		}
	} else {
		query, err = builder.Select(options.Order, options.Limit, options.Offset, options.Filters)
		if err != nil {
			return nil, nil, getBuilderFuncCRUDError("select", err)
		}
	}

	rows, err := c.conn.QueryContext(ctx, query, c.args(sqlfilters.FiltersInterfaces(options.Filters))...)
	if err != nil {
		return nil, nil, getDBFuncCRUDError("query", err)
	}
	defer rows.Close()

	passwordFields := builder.PasswordFields()

	for rows.Next() {
		newObj := newObjFunc()
		err = rows.Scan(objColumnInterfaces(newObj, builder.ColumnFieldNames(), true)...)
		if err != nil {
			return nil, nil, getDBFuncCRUDError("rows scan", err)
		}

		if len(options.VerifyPasswordFields) > 0 {
			hashes := map[string]string{}
			for _, fieldName := range passwordFields {
				if _, ok := options.VerifyPasswordFields[fieldName]; !ok {
					continue
				}
				hashes[fieldName], _ = ObjFieldValue(newObj, fieldName).(string)
			}
			passwordHashesByRow = append(passwordHashesByRow, hashes)
		}

		timeFieldsToUTC(newObj, builder.TimeFields())
		zeroPasswordFields(newObj, passwordFields)

		// If options.RowObjTransformFunc is defined, then call it on the row.
		if options.RowObjTransformFunc != nil {
			returnValue = append(returnValue, options.RowObjTransformFunc(newObj))
			continue
		}

		// Normal append.
		returnValue = append(returnValue, newObj)
	}

	verifiedPasswordFields := map[string]int{}
	if len(options.VerifyPasswordFields) > 0 && len(returnValue) == 1 {
		for fieldName, hash := range passwordHashesByRow[0] {
			if bcrypt.CompareHashAndPassword([]byte(hash), []byte(options.VerifyPasswordFields[fieldName])) != nil {
				verifiedPasswordFields[fieldName] = PassInvalid
				continue
			}
			verifiedPasswordFields[fieldName] = PassOK
		}
	}

	return returnValue, verifiedPasswordFields, nil
}
