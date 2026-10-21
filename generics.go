package gocrud

import (
	"context"
	"errors"
	"strconv"
)

// NotFoundError is returned by Load when there is no object with the given ID.
var NotFoundError = errors.New("object not found")

// UnsupportedOptionError is returned by Get when an option that it does not support is set.
var UnsupportedOptionError = errors.New("option not supported by the generic function, use the CRUD method instead")

// Get returns the objects of type T that match the options. T is a struct type, e.g. Get[User].
// RowObjTransformFunc and VerifyPasswordFields are not supported, use CRUD.Get for them.
func Get[T any](ctx context.Context, c *CRUD, options GetOptions) ([]*T, error) {
	if options.RowObjTransformFunc != nil {
		return nil, getCRUDFuncCRUDError("get", errors.Join(UnsupportedOptionError, errors.New("RowObjTransformFunc")))
	}
	if options.VerifyPasswordFields != nil {
		return nil, getCRUDFuncCRUDError("get", errors.Join(UnsupportedOptionError, errors.New("VerifyPasswordFields")))
	}

	objs, _, err := c.Get(ctx, func() interface{} { return new(T) }, options)
	if err != nil {
		return nil, err
	}

	typedObjs := make([]*T, len(objs))
	for i, obj := range objs {
		typedObjs[i] = obj.(*T)
	}

	return typedObjs, nil
}

// Load returns the object of type T with the given ID, or NotFoundError when there is no such object.
// The related objects of the relation fields named in preload are loaded as well.
func Load[T any](ctx context.Context, c *CRUD, id uint64, preload ...string) (*T, error) {
	obj := new(T)

	output := c.Load(ctx, obj, strconv.FormatUint(id, 10), LoadOptions{Preload: preload})
	if output.Error != nil {
		return nil, output.Error
	}

	if ObjIDValue(obj) == 0 {
		return nil, NotFoundError
	}

	return obj, nil
}

// GetCount returns the number of objects of type T that match the options.
func GetCount[T any](ctx context.Context, c *CRUD, options GetCountOptions) (uint64, error) {
	return c.GetCount(ctx, new(T), options)
}

// UpdateMultiple sets the fields named in values of all the objects of type T that match the options.
func UpdateMultiple[T any](ctx context.Context, c *CRUD, values map[string]interface{}, options UpdateMultipleOptions) error {
	return c.UpdateMultiple(ctx, new(T), values, options)
}

// DeleteMultiple deletes all the objects of type T that match the options, and returns their IDs.
func DeleteMultiple[T any](ctx context.Context, c *CRUD, options DeleteMultipleOptions) ([]uint64, error) {
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
