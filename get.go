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
	Order  []string
	Limit  int
	Offset int
	// After is the cursor of NextCursor: Get returns the objects after the object of the cursor, in the same Order.
	// It cannot be combined with an Offset.
	After                    string
	Filters                  sqlfilters.Filter
	RowObjTransformFunc      func(any) any
	ConvertFiltersFromString bool

	// Preload contains the names of relation fields whose related objects are loaded, e.g. []string{"Customer", "Items"}.
	Preload []string
}

// Get returns the matching objects. Their password fields are empty, see VerifyPasswords.
func (c *CRUD) Get(ctx context.Context, newObjFunc func() any, options GetOptions) ([]any, error) {
	objs, _, err := c.get(ctx, newObjFunc, options, false)
	return objs, err
}

// get returns the matching objects, and the hashes of their password fields, by field name, when withHashes is set.
func (c *CRUD) get(ctx context.Context, newObjFunc func() any, options GetOptions, withHashes bool) ([]any, []map[string]string, error) {
	obj := newObjFunc()

	builder, err := c.builder(obj)
	if err != nil {
		return nil, nil, getBuilderObjectCRUDError(err)
	}
	ctx = c.withQuery(ctx, "Get", builder.TableName())

	if err := c.checkPreload(builder, options.Preload, ""); err != nil {
		return nil, nil, getCRUDFuncCRUDError("preload", err)
	}

	if err := c.checkOrder(builder, options.Order); err != nil {
		return nil, nil, getCRUDFuncCRUDError("order", err)
	}
	options.Order = pageOrder(options.Order, options.Limit > 0 || options.After != "")
	if options.After != "" {
		if options.Offset > 0 {
			return nil, nil, getCRUDFuncCRUDError("cursor", fmt.Errorf("%w: After cannot be combined with an Offset", ErrCursor))
		}
		after, err := cursorFilter(builder, obj, options.Order, options.After)
		if err != nil {
			return nil, nil, getCRUDFuncCRUDError("cursor", err)
		}
		// the values of the cursor are of the types of the fields, so only the other filters are converted from strings
		if options.Filters != nil && options.ConvertFiltersFromString {
			if options.Filters, err = convertFiltersFromString(c.filterTargets(obj), options.Filters); err != nil {
				return nil, nil, err
			}
		}
		options.ConvertFiltersFromString = false
		if options.Filters != nil {
			after = sqlfilters.And(options.Filters, after)
		}
		options.Filters = after
	}

	options.Filters, err = c.prepareFilters(obj, options.Filters, options.ConvertFiltersFromString)
	if err != nil {
		return nil, nil, err
	}

	var returnValue []any
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

	var objs []any
	var pointers []any
	for rows.Next() {
		newObj := newObjFunc()
		pointers = objScanPointers(newObj, builder.ColumnIndexes(), builder.FieldKinds(), pointers)
		err = rows.Scan(pointers...)
		if err != nil {
			return nil, nil, getDBFuncCRUDError("rows scan", err)
		}

		if withHashes {
			hashes := make(map[string]string, len(passwordFields))
			for _, fieldName := range passwordFields {
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

	return returnValue, passwordHashesByRow, nil
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
