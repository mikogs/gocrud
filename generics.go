package gocrud

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Load when there is no object with the given ID.
var ErrNotFound = errors.New("object not found")

// ErrUnsupportedOption is returned by Get when an option that it does not support is set.
var ErrUnsupportedOption = errors.New("option not supported by the generic function, use the CRUD method instead")

// Get returns the objects of type T that match the options. T is a struct type, e.g. Get[User].
// RowObjTransformFunc is not supported, use CRUD.Get for it.
func Get[T any](ctx context.Context, c *CRUD, options GetOptions) ([]*T, error) {
	if options.RowObjTransformFunc != nil {
		return nil, getCRUDFuncCRUDError("get", errors.Join(ErrUnsupportedOption, errors.New("RowObjTransformFunc")))
	}
	objs, err := c.Get(ctx, func() any { return new(T) }, options)
	if err != nil {
		return nil, err
	}

	typedObjs := make([]*T, len(objs))
	for i, obj := range objs {
		typedObjs[i] = obj.(*T)
	}

	return typedObjs, nil
}

// Load returns the object of type T with the given ID, or ErrNotFound when there is no such object.
// The related objects of the relation fields named in preload are loaded as well.
// The ID is a non-negative integer for an ID of type uint64, and a UUID for a string ID.
func Load[T any](ctx context.Context, c *CRUD, id any, preload ...string) (*T, error) {
	obj := new(T)

	if err := c.Load(ctx, obj, id, LoadOptions{Preload: preload}); err != nil {
		return nil, err
	}

	if ObjIDIsZero(obj) {
		return nil, ErrNotFound
	}

	return obj, nil
}

// GetCount returns the number of objects of type T that match the options.
func GetCount[T any](ctx context.Context, c *CRUD, options GetCountOptions) (uint64, error) {
	return c.GetCount(ctx, new(T), options)
}

// UpdateMultiple sets the fields named in values of all the objects of type T that match the options.
func UpdateMultiple[T any](ctx context.Context, c *CRUD, values map[string]any, options UpdateMultipleOptions) error {
	return c.UpdateMultiple(ctx, new(T), values, options)
}

// DeleteMultiple deletes all the objects of type T that match the options, and returns their IDs, of the type of the
// ID field.
func DeleteMultiple[T any](ctx context.Context, c *CRUD, options DeleteMultipleOptions) ([]any, error) {
	return c.DeleteMultiple(ctx, new(T), options)
}

// CreateTable creates the table for objects of type T, if it does not exist.
func CreateTable[T any](ctx context.Context, c *CRUD) error {
	return c.CreateTable(ctx, new(T))
}

// DropTable drops the table for objects of type T, if it exists.
func DropTable[T any](ctx context.Context, c *CRUD) error {
	return c.DropTable(ctx, new(T))
}
