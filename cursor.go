package gocrud

import (
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
	"github.com/mikogs/gocrud/internal/structinfo"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

// ErrCursor is matched by the error of Get with an After cursor that cannot be used, e.g. of another Order, or with an
// Offset, and of NextCursor with an Order of fields that cannot be in a cursor.
var ErrCursor = errors.New("invalid cursor")

// cursor is the position of an object in the order of a page: the values of its order fields.
type cursor struct {
	Order  []string          `json:"o"`
	Values []json.RawMessage `json:"v"`
}

// pageOrder returns the order of the objects: order with ID as the last field, so that the objects with the same
// values of the other fields are always in the same order, and the pages neither miss nor repeat any of them. A page
// without an order is ordered by ID.
func pageOrder(order []string, page bool) []string {
	if len(order) == 0 {
		if page {
			return []string{IDField, "asc"}
		}
		return nil
	}
	for i := 0; i < len(order); i += 2 {
		if order[i] == IDField {
			return order
		}
	}
	return append(slices.Clip(order), IDField, order[len(order)-1])
}

// NextCursor returns the cursor of the page after obj, the last object of a page of Get with order, for GetOptions.After.
// The order fields cannot be nullable, JSON fields, or fields of related objects. The cursor has the values of the
// order fields and the ID of obj, encoded but not encrypted.
func (c *CRUD) NextCursor(obj any, order []string) (string, error) {
	builder, err := c.builder(obj)
	if err != nil {
		return "", getBuilderObjectCRUDError(err)
	}
	if err := c.checkOrder(builder, order); err != nil {
		return "", getCRUDFuncCRUDError("order", err)
	}
	order = pageOrder(order, true)
	if err := checkCursorOrder(builder, order); err != nil {
		return "", getCRUDFuncCRUDError("cursor", err)
	}

	position := cursor{Order: order}
	objValue := reflect.ValueOf(obj).Elem()
	for i := 0; i < len(order); i += 2 {
		field := objValue.FieldByName(order[i])
		value := field.Interface()
		if structinfo.IsValuerType(field.Type()) {
			if value, err = driver.DefaultParameterConverter.ConvertValue(field.Addr().Interface()); err != nil {
				return "", getCRUDFuncCRUDError("cursor", fmt.Errorf("%w: %s: %w", ErrCursor, order[i], err))
			}
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return "", getCRUDFuncCRUDError("cursor", fmt.Errorf("%w: %s: %w", ErrCursor, order[i], err))
		}
		position.Values = append(position.Values, raw)
	}
	data, err := json.Marshal(position)
	if err != nil {
		return "", getCRUDFuncCRUDError("cursor", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// checkCursorOrder returns an error when a field of order cannot be in a cursor: a field of a related object, whose
// object can be missing, or a field that can be NULL, which is not greater or lower than any value, or a JSON field.
func checkCursorOrder(builder *sqlbuilder.Builder, order []string) error {
	for i := 0; i < len(order); i += 2 {
		field := order[i]
		switch {
		case strings.Contains(field, "."):
			return fmt.Errorf("%w: %s is a field of a related object", ErrCursor, field)
		case builder.IsNullable(field):
			return fmt.Errorf("%w: %s can be NULL", ErrCursor, field)
		case slices.Contains(builder.JSONFields(), field):
			return fmt.Errorf("%w: %s is stored as JSON", ErrCursor, field)
		}
	}
	return nil
}

// cursorFilter returns the condition of the objects after the cursor in order: the first order field is after the
// value of the cursor, or it is equal and the second one is after it, and so on. The first order field is also
// compared on its own, so that the database can use its index.
func cursorFilter(builder *sqlbuilder.Builder, obj any, order []string, after string) (sqlfilters.Filter, error) {
	if err := checkCursorOrder(builder, order); err != nil {
		return nil, err
	}
	data, err := base64.RawURLEncoding.DecodeString(after)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCursor, err)
	}
	var position cursor
	if err := json.Unmarshal(data, &position); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCursor, err)
	}
	if !slices.Equal(position.Order, order) || len(position.Values) != len(order)/2 {
		return nil, fmt.Errorf("%w: it is of the order %v, not %v", ErrCursor, position.Order, order)
	}

	objType := reflect.TypeOf(obj).Elem()
	values := make([]any, len(position.Values))
	for i, raw := range position.Values {
		field, _ := objType.FieldByName(order[2*i])
		valueType := field.Type
		if structinfo.IsValuerType(valueType) {
			valueType = structinfo.ValuerValueType(valueType)
		}
		value := reflect.New(valueType)
		if valueType == nil || json.Unmarshal(raw, value.Interface()) != nil {
			return nil, fmt.Errorf("%w: the value of %s", ErrCursor, order[2*i])
		}
		values[i] = value.Elem().Interface()
	}

	positions := make([]sqlfilters.Filter, len(values))
	for i := range values {
		conditions := sqlfilters.Filters{}
		for j := 0; j < i; j++ {
			conditions[order[2*j]] = sqlfilters.OpVal{Op: sqlfilters.OpEqual, Val: values[j]}
		}
		conditions[order[2*i]] = sqlfilters.OpVal{Op: afterOp(order[2*i+1], false), Val: values[i]}
		positions[i] = conditions
	}
	if len(positions) == 1 {
		return positions[0], nil
	}
	first := sqlfilters.Filters{order[0]: {Op: afterOp(order[1], true), Val: values[0]}}
	return sqlfilters.And(first, sqlfilters.Or(positions...)), nil
}

// afterOp returns the operator of the values after a value in an order direction, or equal to it.
func afterOp(direction string, orEqual bool) int {
	desc := strings.EqualFold(direction, "desc")
	switch {
	case desc && orEqual:
		return sqlfilters.OpLowerOrEqual
	case desc:
		return sqlfilters.OpLower
	case orEqual:
		return sqlfilters.OpGreaterOrEqual
	}
	return sqlfilters.OpGreater
}
