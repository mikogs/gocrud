package gocrud

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
)

type InsertMultipleOptions struct {
	// ModifiedBy is the ID of the user who inserts the objects, stored in their CreatedBy and ModifiedBy fields, as in
	// SaveOptions.
	ModifiedBy interface{}
	// ModifiedAt is the time stored in the CreatedAt and ModifiedAt fields, or the current time when it is zero.
	ModifiedAt time.Time
}

// ObjectError is the error of one of the objects of InsertMultiple, e.g. a ValidationError of its fields.
type ObjectError struct {
	// Index is the index of the object in the objects of InsertMultiple.
	Index int
	Err   error
}

func (e *ObjectError) Error() string {
	return fmt.Sprintf("object %d: %s", e.Index, e.Err.Error())
}

func (e *ObjectError) Unwrap() error { return e.Err }

// ErrInsertMultiple is matched by the error of InsertMultiple with objects of different types.
var ErrInsertMultiple = errors.New("objects of different types")

// InsertMultiple inserts the new objects, all of the same type, with INSERT queries of many rows, in a transaction,
// so that either all of them are inserted, or none. Each object is prepared as a new object in Save: it is validated,
// gets a UUID for a string ID, its creation and modification fields, version 1, and its passwords are hashed. The IDs
// of the inserted objects are set on them, and their password fields are empty. When it fails, the objects are as
// they were.
func (c *CRUD) InsertMultiple(ctx context.Context, objs []interface{}, options InsertMultipleOptions) (err error) {
	if len(objs) == 0 {
		return nil
	}
	builder, err := c.builder(objs[0])
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}

	// the objects are restored after an error, as their fields are changed for the queries
	saved := make([]reflect.Value, len(objs))
	for i, obj := range objs {
		if reflect.TypeOf(obj) != reflect.TypeOf(objs[0]) {
			return getCRUDFuncCRUDError("insert multiple", fmt.Errorf("%w: object %d is %T, want %T", ErrInsertMultiple, i, obj, objs[0]))
		}
		value := reflect.ValueOf(obj).Elem()
		saved[i] = reflect.New(value.Type()).Elem()
		saved[i].Set(value)
	}
	defer func() {
		if err != nil {
			for i, obj := range objs {
				reflect.ValueOf(obj).Elem().Set(saved[i])
			}
		}
	}()

	modifiedAt := modificationTime(options.ModifiedAt)
	for i, obj := range objs {
		if err := c.prepareNewObj(builder, obj, modifiedAt, options.ModifiedBy); err != nil {
			return &ObjectError{Index: i, Err: err}
		}
	}

	// a struct with its own insert query is inserted with it, one object at a time
	_, customInsert := objs[0].(insertQueryBuilder)
	err = c.RunInTx(ctx, func(tx *CRUD) error {
		if customInsert {
			for i, obj := range objs {
				if err := tx.insertObj(ctx, builder, obj); err != nil {
					return &ObjectError{Index: i, Err: err}
				}
			}
			return nil
		}
		for first := 0; first < len(objs); first += builder.InsertRowsLimit() {
			if err := tx.insertRows(ctx, builder, objs[first:min(first+builder.InsertRowsLimit(), len(objs))]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, obj := range objs {
		zeroPasswordFields(obj, builder.PasswordIndexes())
	}
	return nil
}

// InsertMultiple inserts the new objects of type T, see CRUD.InsertMultiple.
func InsertMultiple[T any](ctx context.Context, c *CRUD, objs []*T, options InsertMultipleOptions) error {
	list := make([]interface{}, len(objs))
	for i, obj := range objs {
		list[i] = obj
	}
	return c.InsertMultiple(ctx, list, options)
}

// prepareNewObj sets the fields of a new object as Save does before it inserts it. A uint64 ID has to be 0, as the
// database assigns it.
func (c *CRUD) prepareNewObj(builder *sqlbuilder.Builder, obj interface{}, modifiedAt time.Time, modifiedBy interface{}) error {
	ok, violations, err := Validate(obj, nil, c.tagName)
	if err != nil {
		return getValidateObjCRUDError(err)
	}
	if !ok {
		return getObjInvalidCRUDError(violations)
	}

	if builder.HasUUID() {
		if err := setObjUUID(obj, ObjIDIsZero(obj)); err != nil {
			return err
		}
	} else if !ObjIDIsZero(obj) {
		return getCRUDFuncCRUDError("insert multiple", fmt.Errorf("%w: %v, a new object has the ID 0", ErrInvalidID, ObjIDValue(obj)))
	}
	if err := normalizeObjUUIDs(obj, builder.UUIDFields()); err != nil {
		return err
	}
	if builder.HasModificationFields() {
		if err := setModificationFields(obj, true, modifiedAt, modifiedBy); err != nil {
			return err
		}
	}
	if versionField := builder.VersionField(); versionField != "" {
		setVersion(reflect.ValueOf(obj).Elem().FieldByName(versionField), 1)
	}
	return hashObjPasswords(obj, builder.PasswordFields(), map[string]string{})
}

// insertObj inserts a prepared object, with its own insert query when it has one.
func (c *CRUD) insertObj(ctx context.Context, builder *sqlbuilder.Builder, obj interface{}) error {
	query := builder.Insert()
	if inserterImpl, ok := obj.(insertQueryBuilder); ok {
		var err error
		query, err = inserterImpl.InsertQuery()
		if err != nil {
			return getObjFuncCRUDError("insert query", err)
		}
	}
	err := c.conn.QueryRowContext(ctx, query, c.args(objArgs(obj, insertIndexes(builder), builder.FieldKinds()))...).Scan(objIDInterface(obj))
	if err != nil {
		if uniqErr := checkUniqueErr(builder, err); uniqErr != nil {
			return uniqErr
		}
		return getDBFuncCRUDError("query row", err)
	}
	return nil
}

// insertRows inserts prepared objects with one query. The IDs that the database assigns are set on the objects in
// their order: both databases assign increasing IDs to the rows of an INSERT in the order of its values, while the
// order of the rows of RETURNING is not specified.
func (c *CRUD) insertRows(ctx context.Context, builder *sqlbuilder.Builder, objs []interface{}) error {
	indexes := insertIndexes(builder)
	args := make([]interface{}, 0, len(objs)*len(indexes))
	for _, obj := range objs {
		args = append(args, objArgs(obj, indexes, builder.FieldKinds())...)
	}
	query := builder.InsertRows(len(objs))

	if builder.HasUUID() {
		if _, err := c.conn.ExecContext(ctx, query, c.args(args)...); err != nil {
			return insertError(builder, err)
		}
		return nil
	}

	rows, err := c.conn.QueryContext(ctx, query, c.args(args)...)
	if err != nil {
		return insertError(builder, err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]uint64, 0, len(objs))
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return getDBFuncCRUDError("rows scan", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return insertError(builder, err)
	}
	// a trigger can skip rows, e.g. with RAISE(IGNORE) in SQLite
	if len(ids) != len(objs) {
		return getDBFuncCRUDError("insert", fmt.Errorf("%d IDs returned for %d objects", len(ids), len(objs)))
	}
	slices.Sort(ids)
	for i, obj := range objs {
		reflect.ValueOf(obj).Elem().FieldByName(IDField).SetUint(ids[i])
	}
	return nil
}

func insertError(builder *sqlbuilder.Builder, err error) error {
	if uniqErr := checkUniqueErr(builder, err); uniqErr != nil {
		return uniqErr
	}
	return getDBFuncCRUDError("insert", err)
}

// insertIndexes returns the indexes of the fields of the inserted columns: the ID, the first column, only when it is
// a UUID, which is generated before.
func insertIndexes(builder *sqlbuilder.Builder) []int {
	if builder.HasUUID() {
		return builder.ColumnIndexes()
	}
	return builder.ColumnIndexes()[1:]
}
