package registry

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
)

// WithLogger returns a copy of the registry that logs with logger instead of slog.Default().
func (c *CRUD) WithLogger(logger *slog.Logger) *CRUD {
	registry := *c
	registry.logger = logger
	return &registry
}

func (c *CRUD) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

// logError logs the error of an operation. An error caused by the request, e.g. a validation error or a record that
// is not found, is logged at the debug level, as it is returned to the caller, who handles it, and other errors, e.g.
// of the database, at the error level.
func (c *CRUD) logError(ctx context.Context, operation string, msg string, err error, attrs ...slog.Attr) {
	level := slog.LevelError
	if isRequestError(err) {
		level = slog.LevelDebug
	}
	attrs = append(attrs, logger.AttrService(c, operation), logger.AttrError(err))
	c.log().LogAttrs(ctx, level, msg, attrs...)
}

func isRequestError(err error) bool {
	var modelErr *ModelValidationError
	var filterErr *FilterValidationError
	var validationErr *gocrud.ValidationError
	var uniqErr *gocrud.UniqError
	var objectErr *gocrud.ObjectError
	if errors.As(err, &modelErr) || errors.As(err, &filterErr) || errors.As(err, &validationErr) ||
		errors.As(err, &uniqErr) || errors.As(err, &objectErr) {
		return true
	}
	for _, requestErr := range []error{
		ErrInvalidKey, ErrInvalidFilterOp, gocrud.ErrNotFound, gocrud.ErrInvalidID, gocrud.ErrInvalidOrder,
		gocrud.ErrCursor, gocrud.ErrForeignKeyViolation, gocrud.ErrVersionConflict, gocrud.ErrFilterPath,
		gocrud.ErrInFilter, gocrud.ErrMatchFilter, gocrud.ErrPasswordFilter, gocrud.ErrSubquery,
	} {
		if errors.Is(err, requestErr) {
			return true
		}
	}
	return false
}
