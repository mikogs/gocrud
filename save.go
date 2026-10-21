package gocrud

import (
	"context"
	"errors"
	"strings"

	"github.com/lib/pq"
	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	"golang.org/x/crypto/bcrypt"
)

type SaveOptions struct {
	NoInsert   bool
	ModifiedBy uint64
	ModifiedAt int64
}

// Save takes an object, validates its field values, and saves it in the database.
// If ID is not present, then an INSERT will be performed.
// If ID is set, then an "upsert" is performed.
func (c *CRUD) Save(ctx context.Context, obj interface{}, options SaveOptions) error {
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

	objID := ObjIDValue(obj)

	// populate created and modified columns
	hasModificationFields := builder.HasModificationFields()

	if options.ModifiedAt != 0 && options.ModifiedBy != 0 && hasModificationFields {
		SetObjModified(obj, options.ModifiedAt, options.ModifiedBy)
		if objID == 0 {
			SetObjCreated(obj, options.ModifiedAt, options.ModifiedBy)
		}
	}

	// encrypt all password fields
	passFields := builder.PasswordFields()

	for _, passField := range passFields {
		fieldValue := ObjFieldValue(obj, passField)
		fieldStr := fieldValue.(string)
		// Skip fields that are already a bcrypt hash to prevent double-hashing on updates.
		if _, costErr := bcrypt.Cost([]byte(fieldStr)); costErr == nil {
			continue
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(fieldStr), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		SetObjStringField(obj, passField, string(hash))
	}

	// run GetCount on unique fields
	if c.flags&GetCountOnUniq > 0 {
		uniqFields := builder.UniqueFields()
		for _, uniqField := range uniqFields {
			count, err := c.GetCount(ctx, obj,
				GetCountOptions{
					Filters: &sqlfilters.Filters{
						uniqField: sqlfilters.OpVal{
							Op:  sqlfilters.OpEqual,
							Val: ObjFieldValue(obj, uniqField),
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
				return getUniqError(uniqField)
			}
		}
	}

	objIDInterface := ObjIDInterface(obj)
	columnFieldNames := builder.ColumnFieldNames()

	// update
	if objID != 0 {
		// do no try to insert if NoInsert is set
		// TODO: error handling, we should check if object exists - for now nothing happens, UPDATE gets executed and updates nothing
		if options.NoInsert {
			var query string

			// if the object has an UpdateByID method, use it.
			if updateByIDerImpl, ok := obj.(updateByIDQueryBuilder); ok {
				query, err = updateByIDerImpl.UpdateByIDQuery()
				if err != nil {
					return getObjFuncCRUDError("update by id query", err)
				}
			} else {
				query = builder.UpdateByID()
			}
			_, err = c.conn.ExecContext(ctx, query, c.args(append(objColumnInterfaces(obj, columnFieldNames, false), objIDInterface))...)
		} else {
			// try to insert - if ID already exists then try to update it

			var query string

			// if the object has an UpdateByID method, use it.
			if insertOnConflictUpdateerImpl, ok := obj.(insertOnConflictUpdateQueryBuilder); ok {
				query, err = insertOnConflictUpdateerImpl.InsertOnConflictUpdateQuery()
				if err != nil {
					return getObjFuncCRUDError("insert on conflict update query", err)
				}
			} else {
				query = builder.InsertOnConflictUpdate()
			}
			_, err = c.conn.ExecContext(ctx, query, c.args(append(objColumnInterfaces(obj, columnFieldNames, true), objColumnInterfaces(obj, columnFieldNames, false)...))...)
		}

		if err != nil {
			if uniqErr := checkUniqueErr(err); uniqErr != nil {
				return uniqErr
			}
			return getDBFuncCRUDError("query row", err)
		}

		return nil
	}

	// insert
	var query string
	// if the object has an Insert method, use it.
	if inserterImpl, ok := obj.(insertQueryBuilder); ok {
		query, err = inserterImpl.InsertQuery()
		if err != nil {
			return getObjFuncCRUDError("insert query", err)
		}
	} else {
		query = builder.Insert()
	}

	err = c.conn.QueryRowContext(ctx, query, c.args(objColumnInterfaces(obj, columnFieldNames, false))...).Scan(objIDInterface)
	if err != nil {
		if uniqErr := checkUniqueErr(err); uniqErr != nil {
			return uniqErr
		}
		return getDBFuncCRUDError("query row", err)
	}

	return nil
}

// checkUniqueErr returns a *UniqError if err represents a unique-constraint
// violation from either PostgreSQL (pq code 23505) or SQLite
// ("UNIQUE constraint failed: table.column"), otherwise returns nil.
func checkUniqueErr(err error) *UniqError {
	var pgErr *pq.Error
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return getUniqError(pgErr.Constraint)
	}

	msg := err.Error()
	const sqlitePrefix = "UNIQUE constraint failed: "
	if idx := strings.Index(msg, sqlitePrefix); idx >= 0 {
		return getUniqError(msg[idx+len(sqlitePrefix):])
	}

	return nil
}
