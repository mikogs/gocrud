package service

import (
	"errors"

	"github.com/mikogs/gocrud"
)

const (
	LogAttrService = "service"
	LogAttrError   = "err"
)

// NotFoundError is the same error as gocrud.NotFoundError, so that errors.Is works with either of them.
var NotFoundError = gocrud.NotFoundError
var InvalidFilterOpError = errors.New("invalid filter op")

type ModelValidationError struct {
	Err        error
	Violations map[string]string
}

func (e *ModelValidationError) Error() string {
	return e.Err.Error()
}

type FilterValidationError struct {
	Err        error
	Violations map[string]string
}

func (e *FilterValidationError) Error() string {
	return e.Err.Error()
}
