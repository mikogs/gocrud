package gocrud

import (
	"context"
	"maps"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
	"golang.org/x/crypto/bcrypt"
)

type UpdateMultipleOptions struct {
	Filters                 sqlfilters.Filter
	ConvertValuesFromString bool
}

// UpdateMultiple sets the fields of fieldsToUpdate in all the rows that match the filters. The values of password
// fields are hashed with bcrypt, once, so the rows get the same hash. An empty password keeps the stored passwords, as
// in Save. fieldsToUpdate is not changed.
func (c *CRUD) UpdateMultiple(ctx context.Context, obj interface{}, fieldsToUpdate map[string]interface{}, options UpdateMultipleOptions) error {
	builder, err := c.builder(obj)
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}

	fieldsToUpdate = maps.Clone(fieldsToUpdate)
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
	if err := normalizeUUIDValues(fieldsToUpdate, builder.UUIDFields()); err != nil {
		return err
	}

	ok, violations, err := ValidateValues(obj, fieldsToUpdate, c.tagName)
	if err != nil {
		return getValidateObjCRUDError(err)
	}

	if !ok {
		return getObjInvalidCRUDError(violations)
	}

	options.Filters, err = c.prepareFilters(obj, options.Filters, false)
	if err != nil {
		return err
	}

	if err := hashPasswordValues(fieldsToUpdate, builder.PasswordFields()); err != nil {
		return err
	}
	if len(fieldsToUpdate) == 0 {
		return nil
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

	_, err = c.conn.ExecContext(ctx, query, c.args(append(sqlbuilder.MapInterfaces(fieldsToUpdate), sqlfilters.FiltersInterfaces(options.Filters)...))...)
	if err != nil {
		return getDBFuncCRUDError("exec", err)
	}

	return nil
}

// normalizeUUIDValues sets the UUIDs in the values of fields with UUIDs to their standard form, as SQLite compares them
// as text. A value that is not a UUID is a type violation, and nil sets a nullable field to NULL.
func normalizeUUIDValues(values map[string]interface{}, uuidFields []string) error {
	violations := map[string]string{}
	for _, name := range uuidFields {
		value, ok := values[name]
		if !ok || isNilValue(value) {
			continue
		}
		if pointer, ok := value.(*string); ok {
			value = *pointer
		}

		id, err := idValue(true, value)
		if err != nil {
			violations[name] = ViolationType
			continue
		}
		values[name] = id
	}

	if len(violations) > 0 {
		return getObjInvalidCRUDError(violations)
	}
	return nil
}

// hashPasswordValues replaces the values of the password fields with their bcrypt hashes, and removes the empty ones,
// which keep the stored passwords. The values were validated, so they are strings.
func hashPasswordValues(values map[string]interface{}, passwordFields []string) error {
	for _, name := range passwordFields {
		value, ok := values[name]
		if !ok {
			continue
		}

		password := value.(string)
		if password == "" {
			delete(values, name)
			continue
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return passwordHashError(name, err)
		}
		values[name] = string(hash)
	}
	return nil
}
