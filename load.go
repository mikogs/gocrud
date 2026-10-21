package gocrud

import (
	"context"
	"database/sql"
	"strconv"

	"golang.org/x/crypto/bcrypt"
)

type LoadOptions struct {
	Unused bool

	// VerifyPasswordFields maps a struct field name to a plaintext password
	// to verify against the bcrypt hash stored in that field.
	VerifyPasswordFields map[string]string
}

// LoadOutput is returned by Load. PasswordFields holds, for every password
// field that also appears in LoadOptions.VerifyPasswordFields, either PassOK
// or PassInvalid. Keys in VerifyPasswordFields that do not name an actual
// password field are ignored.
type LoadOutput struct {
	Error          error
	PasswordFields map[string]int
}

func (c *CRUD) Load(ctx context.Context, obj interface{}, id string, options LoadOptions) *LoadOutput {
	output := &LoadOutput{
		PasswordFields: map[string]int{},
	}

	idInt, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		output.Error = getConvertIDToIntCRUDError(err)
		return output
	}

	builder, err := c.builder(obj)
	if err != nil {
		output.Error = getBuilderObjectCRUDError(err)
		return output
	}

	var query string

	// if the object has a SelectByID method, use it.
	if selectByIDerImpl, ok := obj.(selectByIDQueryBuilder); ok {
		query, err = selectByIDerImpl.SelectByIDQuery()
		if err != nil {
			output.Error = getObjFuncCRUDError("select by id query", err)
			return output
		}
	} else {
		query = builder.SelectByID()
	}

	err = c.conn.QueryRowContext(ctx, query, idInt).Scan(objColumnInterfaces(obj, builder.ColumnFieldNames(), true)...)
	switch {
	case err == sql.ErrNoRows:
		ZeroObjFields(obj)

		return output

	case err != nil:
		output.Error = getDBFuncCRUDError("query row", err)
		return output

	default:
		passwordFields := builder.PasswordFields()

		for _, fieldName := range passwordFields {
			password, ok := options.VerifyPasswordFields[fieldName]
			if !ok {
				continue
			}

			hash, _ := ObjFieldValue(obj, fieldName).(string)
			if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
				output.PasswordFields[fieldName] = PassInvalid
				continue
			}

			output.PasswordFields[fieldName] = PassOK
		}

		zeroPasswordFields(obj, passwordFields)
		return output
	}
}
