package gocrud

import (
	"context"
	"database/sql"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/mikogs/gocrud/internal/sqlbuilder"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
	"golang.org/x/crypto/bcrypt"
)

type SaveOptions struct {
	// NoInsert updates the object without trying to insert it. When there is no object with the ID, Save returns an
	// error that matches ErrNotFound.
	NoInsert bool
	// ModifiedBy is the ID of the user who saves the object. It is stored in the ModifiedBy field, and in the
	// CreatedBy field when the object is created: a non-negative integer for fields of type uint64, and a string
	// for fields of type string. nil stores the zero value.
	ModifiedBy interface{}
	// ModifiedAt is the time of the change, stored like ModifiedBy. When it is zero, the current time is used.
	ModifiedAt time.Time
}

// Save takes an object, validates its field values, and saves it in the database.
// If ID is not present, then an INSERT will be performed.
// If ID is set, then an "upsert" is performed.
// Password fields are hashed with bcrypt. An empty password field keeps the stored password of an object that is
// updated, and leaves a new object without a password that can be verified. The password fields are empty after the
// object is saved.
func (c *CRUD) Save(ctx context.Context, obj interface{}, options SaveOptions) (err error) {
	builder, err := c.builder(obj)
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}

	ok, violations, err := Validate(obj, nil, c.tagName)
	if err != nil {
		return getValidateObjCRUDError(err)
	}

	if !ok {
		return getObjInvalidCRUDError(violations)
	}

	isNew := ObjIDIsZero(obj)
	if builder.HasUUID() {
		if err := setObjUUID(obj, isNew); err != nil {
			return err
		}
	}
	if err := normalizeObjUUIDs(obj, builder.UUIDFields()); err != nil {
		return err
	}
	objID := ObjIDValue(obj)

	if builder.HasModificationFields() {
		modifiedAt := options.ModifiedAt
		if modifiedAt.IsZero() {
			// databases store times with a precision of up to microseconds
			modifiedAt = time.Now().Truncate(time.Microsecond)
		}
		modifiedAt = modifiedAt.UTC()

		if err := setObjModified(obj, modifiedAt, options.ModifiedBy); err != nil {
			return getCRUDFuncCRUDError("set modification fields", err)
		}

		// Creation fields are never updated in the database. They are set when the object is inserted, and when
		// they are empty, as "upsert" inserts the object when it does not exist in the database.
		if isNew || objFieldValue(obj, "CreatedAt").(time.Time).IsZero() {
			if err := setObjCreated(obj, modifiedAt, options.ModifiedBy); err != nil {
				return getCRUDFuncCRUDError("set creation fields", err)
			}
		}
	}

	// The passwords are hashed for the query, and the password fields are empty after the object is saved, as after
	// Load, so that saving the object again does not change the passwords. After an error, they have the passwords
	// again, so that saving the object again saves them.
	passFields := builder.PasswordFields()
	passwords := map[string]string{}
	defer func() {
		if err == nil {
			zeroPasswordFields(obj, builder.PasswordIndexes())
			return
		}
		for passField, password := range passwords {
			setObjStringField(obj, passField, password)
		}
	}()

	for _, passField := range passFields {
		password := objFieldValue(obj, passField).(string)
		// an empty password is not hashed: an update keeps the stored password, and a new object has no password
		// that can be verified
		if password == "" {
			continue
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return passwordHashError(passField, err)
		}

		passwords[passField] = password
		setObjStringField(obj, passField, string(hash))
	}

	if c.flags&GetCountOnUniq > 0 {
		uniqFields := builder.UniqueFields()
		for _, uniqField := range uniqFields {
			// NULL values are not equal to each other, so they are never duplicates
			uniqValue := objFieldValue(obj, uniqField)
			if uniqValue == nil {
				continue
			}

			count, err := c.GetCount(ctx, obj,
				GetCountOptions{
					Filters: &sqlfilters.Filters{
						uniqField: sqlfilters.OpVal{
							Op:  sqlfilters.OpEqual,
							Val: uniqValue,
						},
						"ID": sqlfilters.OpVal{
							Op:  sqlfilters.OpNotEqual,
							Val: objID,
						},
					},
				},
			)
			if err != nil {
				return getCRUDFuncCRUDError("get count", err)
			}

			if count > 0 {
				return getUniqError([]string{uniqField}, "")
			}
		}
	}

	idPointer := objIDInterface(obj)
	columnIndexes := builder.ColumnIndexes()
	updateIndexes := builder.UpdateIndexes()

	if !isNew {
		var result sql.Result

		if options.NoInsert {
			var query string
			if updateByIDerImpl, ok := obj.(updateByIDQueryBuilder); ok {
				query, err = updateByIDerImpl.UpdateByIDQuery()
				if err != nil {
					return getObjFuncCRUDError("update by id query", err)
				}
			} else {
				query = builder.UpdateByID()
			}
			result, err = c.conn.ExecContext(ctx, query, c.args(append(objArgs(obj, updateIndexes, builder.FieldKinds()), idPointer))...)
		} else {
			var query string
			if insertOnConflictUpdateerImpl, ok := obj.(insertOnConflictUpdateQueryBuilder); ok {
				query, err = insertOnConflictUpdateerImpl.InsertOnConflictUpdateQuery()
				if err != nil {
					return getObjFuncCRUDError("insert on conflict update query", err)
				}
			} else {
				query = builder.InsertOnConflictUpdate()
			}
			_, err = c.conn.ExecContext(ctx, query, c.args(append(objArgs(obj, columnIndexes, builder.FieldKinds()), objArgs(obj, updateIndexes, builder.FieldKinds())...))...)
		}

		if err != nil {
			if uniqErr := checkUniqueErr(builder, err); uniqErr != nil {
				return uniqErr
			}
			return getDBFuncCRUDError("query row", err)
		}

		if options.NoInsert {
			updated, err := result.RowsAffected()
			if err != nil {
				return getDBFuncCRUDError("rows affected", err)
			}
			if updated == 0 {
				return getCRUDFuncCRUDError("update", ErrNotFound)
			}
		}

		return nil
	}

	var query string
	if inserterImpl, ok := obj.(insertQueryBuilder); ok {
		query, err = inserterImpl.InsertQuery()
		if err != nil {
			return getObjFuncCRUDError("insert query", err)
		}
	} else {
		query = builder.Insert()
	}

	// the ID, the first column, is inserted only when it is a UUID, which is generated before
	insertIndexes := columnIndexes
	if !builder.HasUUID() {
		insertIndexes = columnIndexes[1:]
	}
	err = c.conn.QueryRowContext(ctx, query, c.args(objArgs(obj, insertIndexes, builder.FieldKinds()))...).Scan(idPointer)
	if err != nil {
		if uniqErr := checkUniqueErr(builder, err); uniqErr != nil {
			return uniqErr
		}
		return getDBFuncCRUDError("query row", err)
	}

	return nil
}

// setObjUUID sets the ID of a new object to a new UUIDv7, which starts with the time, so that new rows are added at
// the end of the index. The ID of an existing object has to be a UUID, and is set to its standard form, as SQLite
// compares UUIDs as text.
func setObjUUID(obj interface{}, isNew bool) error {
	idField := reflect.ValueOf(obj).Elem().FieldByName(IDField)

	if isNew {
		id, err := uuid.NewV7()
		if err != nil {
			return getCRUDFuncCRUDError("generate uuid", err)
		}
		idField.SetString(id.String())
		return nil
	}

	id, err := uuid.Parse(idField.String())
	if err != nil {
		return getObjInvalidCRUDError(map[string]string{IDField: ViolationType})
	}
	idField.SetString(id.String())

	return nil
}

// normalizeObjUUIDs sets the UUIDs in the foreign keys of obj to their standard form, as SQLite compares them as text.
// A value that is not a UUID, e.g. an empty string, is a type violation. Use a *string for a key that can be empty.
func normalizeObjUUIDs(obj interface{}, uuidFields []string) error {
	objValue := reflect.ValueOf(obj).Elem()

	violations := map[string]string{}
	for _, name := range uuidFields {
		field := objValue.FieldByName(name)
		if name == IDField || (field.Kind() == reflect.Pointer && field.IsNil()) {
			continue
		}
		if field.Kind() == reflect.Pointer {
			field = field.Elem()
		}

		id, err := idValue(true, field.String())
		if err != nil {
			violations[name] = ViolationType
			continue
		}
		field.SetString(id.(string))
	}

	if len(violations) > 0 {
		return getObjInvalidCRUDError(violations)
	}
	return nil
}

func checkUniqueErr(builder *sqlbuilder.Builder, err error) *UniqError {
	key, ok := uniqueViolationKey(err)
	if !ok {
		return nil
	}
	return getUniqError(builder.UniqueKeyFields(key), key)
}
