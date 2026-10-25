package gocrud

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/mikogs/gocrud/internal/sqlbuilder/where"
	"github.com/mikogs/gocrud/internal/structinfo"
)

// CRUDError is the error of most operations. Op describes the step that failed, e.g. "database query row", for the
// message of the error, and can change in any version, so check the errors with errors.Is and errors.As instead, e.g.
// errors.Is(err, gocrud.ErrNotFound).
type CRUDError struct {
	Op  string
	Err error
}

func (e *CRUDError) Error() string {
	return e.Op + ": " + e.Err.Error()
}

func (e *CRUDError) Unwrap() error {
	return e.Err
}

type ValidationError struct {
	Violations map[string]string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed with violations: %v", e.Violations)
}

type UniqError struct {
	// Field is the first of Fields, or the name of the constraint when it is not one that gocrud creates, e.g. a
	// constraint created by hand.
	Field string
	// Fields are the fields of the unique column or index, in its order.
	Fields []string
}

func (e *UniqError) Error() string {
	return "uniq failed for field: " + e.Field
}

var getBuilderObjectCRUDError = func(err error) *CRUDError {
	return &CRUDError{
		Op:  "get builder object",
		Err: err,
	}
}

// ErrInvalidID is matched by the error of an operation that gets an ID of the wrong type, e.g. a number for a struct
// with a UUID, or a string that is not a UUID.
var ErrInvalidID = errors.New("invalid id")

// ErrInvalidOrder is matched by the error of Get with an Order that has a field that does not exist, a path through a
// field that is not a belongs-to relation, or an odd number of elements.
var ErrInvalidOrder = errors.New("invalid order")

// ErrFilterPath is matched by the error of an operation with a filter on a path through a field that is not a relation
// field, e.g. Status.Name.
var ErrFilterPath = where.ErrRelationPath

// ErrSubquery is matched by the error of an operation with a filter that has a subquery that cannot be used, e.g. on a
// field that is not stored, or with an operator other than OpIn.
var ErrSubquery = where.ErrSubquery

// The errors of filters that cannot be built, e.g. a raw filter with a wrong number of values.
var (
	// ErrInFilter is matched by the error of an OpIn condition whose value is not a slice, an array or a subquery.
	ErrInFilter = where.ErrInFilter
	// ErrMatchFilter is matched by the error of an OpMatch condition with a database that does not support it, SQLite.
	ErrMatchFilter = where.ErrMatchFilter
	// ErrRawFilter is matched by the error of a raw filter whose number of ? is not its number of values.
	ErrRawFilter = where.ErrRawFilter
	// ErrPasswordFilter is matched by the error of a raw filter or an order with a password field, e.g. .Password,
	// whose hashes it would reveal. A condition on a password field is a validation error, see ViolationPassword.
	ErrPasswordFilter = where.ErrPasswordField
)

// The errors of structs that cannot be used, which every operation on such a struct returns, see Struct in the docs.
var (
	// ErrNotStructPointer is matched by the error of an operation with an obj that is not a pointer to a struct.
	ErrNotStructPointer = structinfo.ErrNotStructPointer
	// ErrIDField is matched by the error of a struct whose first field is not ID, of type uint64 or string.
	ErrIDField = structinfo.ErrIDField
	// ErrNoColumns is matched by the error of a struct without a stored field other than ID.
	ErrNoColumns = structinfo.ErrNoColumns
	// ErrPasswordField is matched by the error of a struct with a field tagged with "pass" that is not a string.
	ErrPasswordField = structinfo.ErrPasswordField
	// ErrModificationFields is matched by the error of a struct whose CreatedBy and ModifiedBy are of different types.
	ErrModificationFields = structinfo.ErrModificationFields
	// ErrVersionField is matched by the error of a struct with more than one field tagged with "version", or with one
	// that is not an integer, and of UpdateMultiple with a value of the version field.
	ErrVersionField = structinfo.ErrVersionField
	// ErrRelation is matched by the error of a struct with a relation field that cannot be used, e.g. with a key field
	// that does not exist.
	ErrRelation = structinfo.ErrRelation
	// ErrForeignKey is matched by the error of a struct with an fk or on_delete tag that cannot be used.
	ErrForeignKey = structinfo.ErrForeignKey
	// ErrColumnType is matched by the error of a struct with an invalid type tag, e.g. type:varchar(20);, or with a field
	// of a Valuer type whose zero value is NULL, which needs a type tag, as the type of its column is not known.
	ErrColumnType = structinfo.ErrColumnType
)

// ErrQueryMethod is matched by the error of an operation on a struct that has a query method, e.g. SelectQuery,
// with a signature other than the one that gocrud calls.
var ErrQueryMethod = errors.New("invalid query method")

// ErrVersionConflict is matched by the error of Save of an object whose version field has another value than the
// stored one, as the object was changed since it was loaded, see the "version" tag.
var ErrVersionConflict = errors.New("version conflict")

// ErrForeignKeyViolation is matched by the errors of queries that violate a foreign key, e.g. a save of an object
// that references an object that does not exist, or a delete of an object that is referenced.
var ErrForeignKeyViolation = errors.New("foreign key violation")

var getDBFuncCRUDError = func(action string, err error) *CRUDError {
	if isForeignKeyViolation(err) {
		err = fmt.Errorf("%w: %w", ErrForeignKeyViolation, err)
	}

	return &CRUDError{
		Op:  "database " + action,
		Err: err,
	}
}

func isForeignKeyViolation(err error) bool {
	if state, _ := postgresError(err); state != "" {
		return state == "23503"
	}
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

var regexpSQLiteErrorCode = regexp.MustCompile(` \([0-9]+\)$`)

// uniqueViolationKey returns whether err is a violation of a unique constraint or index, and how the database names
// it: by the name of the constraint in PostgreSQL, e.g. "user_email_key", and by the columns in SQLite, e.g.
// "user.email", which is the key of Builder.UniqueKeyFields.
func uniqueViolationKey(err error) (string, bool) {
	if state, constraint := postgresError(err); state != "" {
		return constraint, state == "23505"
	}

	msg := err.Error()
	const sqlitePrefix = "UNIQUE constraint failed: "
	if idx := strings.Index(msg, sqlitePrefix); idx >= 0 {
		// some drivers add the error code at the end, e.g. " (2067)"
		return regexpSQLiteErrorCode.ReplaceAllString(msg[idx+len(sqlitePrefix):], ""), true
	}
	return "", false
}

// postgresError returns the SQLSTATE code and the constraint of an error of PostgreSQL, or empty strings for other
// errors. They are read by the method and the fields that the errors of the drivers have, Constraint in lib/pq and
// ConstraintName in pgx, so that gocrud works with any driver without depending on it.
func postgresError(err error) (string, string) {
	var stateErr interface{ SQLState() string }
	if !errors.As(err, &stateErr) {
		return "", ""
	}

	constraint := ""
	if value := reflect.Indirect(reflect.ValueOf(stateErr)); value.Kind() == reflect.Struct {
		for _, name := range []string{"ConstraintName", "Constraint"} {
			if field := value.FieldByName(name); field.Kind() == reflect.String {
				constraint = field.String()
				break
			}
		}
	}
	return stateErr.SQLState(), constraint
}

var getBuilderFuncCRUDError = func(action string, err error) *CRUDError {
	return &CRUDError{
		Op:  "builder " + action,
		Err: err,
	}
}

var getCRUDFuncCRUDError = func(action string, err error) *CRUDError {
	return &CRUDError{
		Op:  "crud " + action,
		Err: err,
	}
}
var getObjFuncCRUDError = func(action string, err error) *CRUDError {
	return &CRUDError{
		Op:  "object " + action,
		Err: err,
	}
}

var getValidateObjCRUDError = func(err error) *CRUDError {
	return &CRUDError{
		Op:  "validate object",
		Err: err,
	}
}

var getObjInvalidCRUDError = func(violations map[string]string) *CRUDError {
	return &CRUDError{
		Op: "validate object violations",
		Err: &ValidationError{
			Violations: violations,
		},
	}
}

// getUniqError returns the error of a violation of the unique column or index of fields, or of a constraint that is
// not one of the struct when fields is empty.
var getUniqError = func(fields []string, constraint string) *UniqError {
	if len(fields) == 0 {
		return &UniqError{Field: constraint}
	}
	return &UniqError{Field: fields[0], Fields: fields}
}

var getStructFieldValueFromStringCRUDError = func() *CRUDError {
	return &CRUDError{
		Op:  "builder struct field value from string",
		Err: errors.New("error converting string to field type"),
	}
}
