package gocrud

import (
	"errors"
	"fmt"
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
	return "uniq %s failed for field: " + e.Field
}

var getBuilderObjectCRUDError = func(err error) *CRUDError {
	return &CRUDError{
		Op:  "get builder object",
		Err: err,
	}
}
var getDBFuncCRUDError = func(action string, err error) *CRUDError {
	return &CRUDError{
		Op:  "database " + action,
		Err: err,
	}
}
var getCascadingDeleteCRUDError = func(err error) *CRUDError {
	return &CRUDError{
		Op:  "cascading delete",
		Err: err,
	}
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

var getUpdateFieldFromTagsCRUDError = func() *CRUDError {
	return &CRUDError{
		Op:  "update field from tags",
		Err: errors.New("missing"),
	}
}

var getConvertIDToIntCRUDError = func(err error) *CRUDError {
	return &CRUDError{
		Op:  "convert id to int",
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
