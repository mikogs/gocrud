package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/mikogs/gocrud"
	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/logger"
)

var (
	InvalidKeyError = errors.New("invalid key")
)

type CRUD struct {
	registry map[string]func() interface{}
	crud     *gocrud.CRUD
}

func New(registry map[string]func() interface{}, dbConn *sql.DB, dialect string) *CRUD {
	return &CRUD{
		registry: registry,
		crud:     gocrud.New(dbConn, gocrud.Options{Dialect: dialect}),
	}
}

// CreateTables creates a table for every constructor in the registry, except
// for structs whose type name contains an underscore. gocrud derives table
// names from the part of the struct name before the first underscore, so a
// struct like User_SaveLogIn is a projection mapping onto the table another
// struct (User) already owns, exposing only a subset of its columns —
// creating a table from that projection would define it with the wrong
// (partial) column set. Since registry iteration order is unspecified, this
// keeps CreateTables deterministic regardless of how many projections are
// registered alongside the owning struct.
// The tables are created in the order of the registry keys, except that the
// tables referenced by foreign keys are created before the tables that
// reference them.
func (c *CRUD) CreateTables(ctx context.Context) error {
	logAttrService := logger.AttrService(c, "CreateTables")

	keys, err := c.tableKeys()
	if err != nil {
		slog.Error("error ordering tables", logAttrService, logger.AttrError(err))
		return err
	}

	for _, key := range keys {
		err = c.crud.CreateTable(ctx, c.registry[key]())
		if err != nil {
			slog.Error("error creating table", logAttrService, logger.AttrError(err))
			return err
		}
	}
	return nil
}

func (c *CRUD) New(key string) interface{} {
	constructor, ok := c.registry[key]
	if !ok {
		return nil
	}
	return constructor()
}

func (c *CRUD) ID(obj interface{}) uint64 {
	return gocrud.ObjIDValue(obj)
}

// PasswordFieldNames returns the struct field names tagged as passwords for
// the constructor registered at key. Returns nil if the key is unknown or
// the struct has no password fields.
func (c *CRUD) PasswordFieldNames(key string) []string {
	constructor, ok := c.registry[key]
	if !ok {
		return nil
	}
	return c.crud.PasswordFieldsFor(constructor())
}

// Registry returns the registered path keys.
func (c *CRUD) Registry() []string {
	keys := make([]string, 0, len(c.registry))
	for k := range c.registry {
		keys = append(keys, k)
	}
	return keys
}

// buildFilters converts the string-keyed filter maps coming from HTTP layers
// into a sqlfilters.Filters value ready for the CRUD layer.
func buildFilters(filterVals, filterOps map[string]string, logAttr slog.Attr) (*sqlfilters.Filters, error) {
	filters := sqlfilters.Filters{}
	violations := map[string]string{}

	for name, value := range filterVals {
		op := sqlfilters.OpEqual
		if filterOp, ok := filterOps[name]; ok {
			var err error
			op, err = Op(filterOp)
			if err != nil {
				slog.Error("invalid filter op", logAttr, slog.String("op", filterOp))
				violations[name] = gocrud.ViolationType
			}
		}
		filters[name] = sqlfilters.OpVal{Op: op, Val: value}
	}

	if len(violations) > 0 {
		slog.Error("invalid filter ops", logAttr)
		return nil, &FilterValidationError{
			Err:        errors.New("invalid filter ops"),
			Violations: violations,
		}
	}

	return &filters, nil
}

func Op(op string) (int, error) {
	switch op {
	case "eq":
		return sqlfilters.OpEqual, nil
	case "ne":
		return sqlfilters.OpNotEqual, nil
	case "lt":
		return sqlfilters.OpLower, nil
	case "le":
		return sqlfilters.OpLowerOrEqual, nil
	case "gt":
		return sqlfilters.OpGreater, nil
	case "ge":
		return sqlfilters.OpGreaterOrEqual, nil
	case "like":
		return sqlfilters.OpLike, nil
	case "match":
		return sqlfilters.OpMatch, nil
	case "bit":
		return sqlfilters.OpBit, nil
	case "in":
		return sqlfilters.OpIn, nil
	default:
		return 0, InvalidFilterOpError
	}
}
