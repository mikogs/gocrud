package gocrud

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
	"github.com/mikogs/gocrud/internal/structinfo"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type GetOptions struct {
	Order                    []string
	Limit                    int
	Offset                   int
	Filters                  sqlfilters.Filter
	RowObjTransformFunc      func(interface{}) interface{}
	ConvertFiltersFromString bool

	// VerifyPasswordFields maps the names of password fields to the passwords to verify with their hashes, when exactly
	// one object matches.
	VerifyPasswordFields map[string]string

	// Preload contains the names of relation fields whose related objects are loaded, e.g. []string{"Customer", "Items"}.
	Preload []string
}

// Get returns the matching objects, and PassOK or PassInvalid for each password field in
// options.VerifyPasswordFields when exactly one object matches. Other keys of VerifyPasswordFields are ignored.
func (c *CRUD) Get(ctx context.Context, newObjFunc func() interface{}, options GetOptions) ([]interface{}, map[string]int, error) {
	obj := newObjFunc()

	builder, err := c.builder(obj)
	if err != nil {
		return nil, nil, getBuilderObjectCRUDError(err)
	}

	if err := c.checkPreload(builder, options.Preload, ""); err != nil {
		return nil, nil, getCRUDFuncCRUDError("preload", err)
	}

	if err := c.checkOrder(builder, options.Order); err != nil {
		return nil, nil, getCRUDFuncCRUDError("order", err)
	}

	options.Filters, err = c.prepareFilters(obj, options.Filters, options.ConvertFiltersFromString)
	if err != nil {
		return nil, nil, err
	}

	var returnValue []interface{}
	var passwordHashesByRow []map[string]string

	var query string

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
	defer func() { _ = rows.Close() }()

	passwordFields := builder.PasswordFields()

	var objs []interface{}
	var pointers []interface{}
	for rows.Next() {
		newObj := newObjFunc()
		pointers = objScanPointers(newObj, builder.ColumnIndexes(), builder.NamedBoolColumns(), pointers)
		err = rows.Scan(pointers...)
		if err != nil {
			return nil, nil, getDBFuncCRUDError("rows scan", err)
		}

		if len(options.VerifyPasswordFields) > 0 {
			hashes := map[string]string{}
			for _, fieldName := range passwordFields {
				if _, ok := options.VerifyPasswordFields[fieldName]; !ok {
					continue
				}
				hashes[fieldName], _ = objFieldValue(newObj, fieldName).(string)
			}
			passwordHashesByRow = append(passwordHashesByRow, hashes)
		}

		timeFieldsToUTC(newObj, builder.TimeIndexes())
		zeroPasswordFields(newObj, builder.PasswordIndexes())

		objs = append(objs, newObj)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, getDBFuncCRUDError("rows", err)
	}

	// Related objects are loaded with other queries, which cannot run on the same connection, e.g. in a transaction,
	// while rows are open. Next closes the rows when it returns false, so this only makes sure they are closed.
	if err = rows.Close(); err != nil {
		return nil, nil, getDBFuncCRUDError("rows close", err)
	}

	err = c.preload(ctx, builder, objs, options.Preload)
	if err != nil {
		return nil, nil, err
	}

	for _, obj := range objs {
		if options.RowObjTransformFunc != nil {
			returnValue = append(returnValue, options.RowObjTransformFunc(obj))
			continue
		}

		returnValue = append(returnValue, obj)
	}

	// the passwords are compared also when they are not verified, so that the time of Get does not show how many
	// objects match the filters, e.g. whether a user with the e-mail of a login exists
	verifiedPasswordFields := map[string]int{}
	if len(options.VerifyPasswordFields) > 0 {
		if len(returnValue) == 1 {
			for fieldName, hash := range passwordHashesByRow[0] {
				verifiedPasswordFields[fieldName] = verifyPassword(hash, options.VerifyPasswordFields[fieldName])
			}
		} else {
			verifyWithoutObject(passwordFields, options.VerifyPasswordFields)
		}
	}

	return returnValue, verifiedPasswordFields, nil
}

// checkOrder checks that the fields of order are stored fields of the struct of builder, or paths through belongs-to
// relations to such fields of related structs, e.g. Customer.Name.
func (c *CRUD) checkOrder(builder *sqlbuilder.Builder, order []string) error {
	if len(order)%2 != 0 {
		return fmt.Errorf("%w: %d elements, want pairs of a field and a direction", ErrInvalidOrder, len(order))
	}

	for i := 0; i < len(order); i += 2 {
		current := builder
		segments := strings.Split(order[i], ".")
		for _, segment := range segments[:len(segments)-1] {
			relation := findRelation(current, segment)
			if relation == nil {
				return fmt.Errorf("%w: %s: %s is not a relation field", ErrInvalidOrder, order[i], segment)
			}
			if relation.Kind != structinfo.BelongsTo {
				return fmt.Errorf("%w: %s: %s is a has-many relation, which has many values", ErrInvalidOrder, order[i], segment)
			}

			var err error
			current, err = c.builder(reflect.New(relation.Type).Interface())
			if err != nil {
				return getBuilderObjectCRUDError(err)
			}
		}

		if !slices.Contains(current.ColumnFieldNames(), segments[len(segments)-1]) {
			return fmt.Errorf("%w: %s is not a stored field", ErrInvalidOrder, order[i])
		}
		// the order of the objects would reveal the hashes
		if slices.Contains(current.PasswordFields(), segments[len(segments)-1]) {
			return fmt.Errorf("%w: %s is a password field", ErrInvalidOrder, order[i])
		}
	}

	return nil
}
