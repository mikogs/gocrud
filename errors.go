package gocrud

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

type CRUDError struct {
	Op  string
	Tag string
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
	Field string
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
var ErrFilterPath = errors.New("invalid filter path")

// ErrSubquery is matched by the error of an operation with a filter that has a subquery that cannot be used, e.g. on a
// field that is not stored, or with an operator other than OpIn.
var ErrSubquery = errors.New("invalid subquery")

// ErrQueryMethod is matched by the error of an operation on a struct that has a query method, e.g. SelectQuery,
// with a signature other than the one that gocrud calls.
var ErrQueryMethod = errors.New("invalid query method")

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

// isForeignKeyViolation checks if err is a foreign key violation in PostgreSQL (pq code 23503)
// or SQLite ("FOREIGN KEY constraint failed").
func isForeignKeyViolation(err error) bool {
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}

	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
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

var getUniqError = func(field string) *UniqError {
	return &UniqError{
		Field: field,
	}
}

var getStructFieldValueFromStringCRUDError = func() *CRUDError {
	return &CRUDError{
		Op:  "builder struct field value from string",
		Err: errors.New("error converting string to field type"),
	}
}
