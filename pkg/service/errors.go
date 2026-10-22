package service

import (
	"errors"

	"github.com/mikogs/gocrud"
)

// ErrNotFound is the same error as gocrud.ErrNotFound, so that errors.Is works with either of them.
var ErrNotFound = gocrud.ErrNotFound
var ErrInvalidFilterOp = errors.New("invalid filter op")

// ErrInvalidID is the same error as gocrud.ErrInvalidID, returned for an ID of the wrong type, e.g. a number for a
// struct with a UUID, or a string that is not a UUID.
var ErrInvalidID = gocrud.ErrInvalidID

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
