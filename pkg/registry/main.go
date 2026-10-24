package registry

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"reflect"
	"sort"
	"strings"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/logger"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

var (
	ErrInvalidKey = errors.New("invalid key")
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

// CreateTables creates the tables of the structs in the registry, in the order of the registry keys, except that the
// tables referenced by foreign keys are created first. A struct whose name contains an underscore, e.g. User_SaveLogIn,
// is a projection of the table of another struct, User, so its table is not created.
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

// ID returns the ID of obj: a uint64, or a string with a UUID.
func (c *CRUD) ID(obj interface{}) interface{} {
	return gocrud.ObjIDValue(obj)
}

// PasswordFieldNames returns the names of the password fields of the struct of key, or nil for a key that is not
// registered.
func (c *CRUD) PasswordFieldNames(key string) []string {
	constructor, ok := c.registry[key]
	if !ok {
		return nil
	}
	return c.crud.PasswordFieldsFor(constructor())
}

// RelatedType returns the type of the related struct of the relation field of obj, and false when field is not a
// relation field, see gocrud.CRUD.RelatedType.
func (c *CRUD) RelatedType(obj interface{}, field string) (reflect.Type, bool) {
	return c.crud.RelatedType(obj, field)
}

// Registry returns the registered path keys.
func (c *CRUD) Registry() []string {
	keys := make([]string, 0, len(c.registry))
	for k := range c.registry {
		keys = append(keys, k)
	}
	return keys
}

// Filters are the filters of List and Num, as strings, e.g. from the query of a request. The values are converted to
// the types of the fields, and the operators are the names that Op accepts, with "eq" when an operator is not set.
type Filters struct {
	// Vals and Ops are keyed by field name, or by a path through relation fields, e.g. "Customer.Country". A key can
	// also name a group, after a tilde, e.g. "Name~q". The conditions of a group are joined with OR, and the groups and
	// the conditions without a group are joined with AND. A group with one condition is a condition joined with AND,
	// so e.g. "Price~min" and "Price~max" are a range.
	Vals map[string]string
	Ops  map[string]string

	// ScopeVals and ScopeOps are conditions that every record has to match, e.g. that it belongs to the user. They
	// are keyed by field name, or by a path, and joined with AND with all the other conditions, which cannot match
	// other records.
	ScopeVals map[string]string
	ScopeOps  map[string]string
}

// buildFilter returns the filter of Filters for the CRUD layer.
func buildFilter(filters Filters, logAttr slog.Attr) (sqlfilters.Filter, error) {
	violations := map[string]string{}

	scope := sqlfilters.Filters{}
	for name, value := range filters.ScopeVals {
		if !isFilterPath(name) {
			slog.Error("invalid filter name", logAttr, slog.String("name", name))
			violations[name] = gocrud.ViolationType
			continue
		}
		addCondition(scope, name, value, filters.ScopeOps[name], violations, logAttr)
	}

	keys := make([]string, 0, len(filters.Vals))
	for key := range filters.Vals {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	conditions := sqlfilters.Filters{}
	groups := map[string][]sqlfilters.Filter{}
	for _, key := range keys {
		value := filters.Vals[key]
		name, group, hasGroup := strings.Cut(key, "~")
		if !isFilterPath(name) || (hasGroup && (group == "" || strings.Contains(group, "~"))) {
			slog.Error("invalid filter name", logAttr, slog.String("name", key))
			violations[key] = gocrud.ViolationType
			continue
		}

		if !hasGroup {
			addCondition(conditions, name, value, filters.Ops[key], violations, logAttr)
			continue
		}

		condition := sqlfilters.Filters{}
		addCondition(condition, name, value, filters.Ops[key], violations, logAttr)
		groups[group] = append(groups[group], condition)
	}

	if len(violations) > 0 {
		slog.Error("invalid filter ops", logAttr)
		return nil, &FilterValidationError{
			Err:        errors.New("invalid filter ops"),
			Violations: violations,
		}
	}

	groupNames := make([]string, 0, len(groups))
	for group := range groups {
		groupNames = append(groupNames, group)
	}
	sort.Strings(groupNames)

	filter := sqlfilters.And(scope, conditions)
	for _, group := range groupNames {
		filter.Filters = append(filter.Filters, sqlfilters.Or(groups[group]...))
	}

	return filter, nil
}

// isFilterPath checks if name is a field name, or a path of field names separated with dots, e.g. Customer.Country.
func isFilterPath(name string) bool {
	for _, segment := range strings.Split(name, ".") {
		if segment == "" || strings.Contains(segment, "~") {
			return false
		}
	}
	return true
}

// addCondition adds the condition on the field name to filters, or a violation when the operator is invalid.
func addCondition(filters sqlfilters.Filters, name string, value string, filterOp string, violations map[string]string, logAttr slog.Attr) {
	op := sqlfilters.OpEqual
	if filterOp != "" {
		var err error
		op, err = Op(filterOp)
		if err != nil {
			slog.Error("invalid filter op", logAttr, slog.String("op", filterOp))
			violations[name] = gocrud.ViolationType
		}
	}
	filters[name] = sqlfilters.OpVal{Op: op, Val: value}
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
		return 0, ErrInvalidFilterOp
	}
}
