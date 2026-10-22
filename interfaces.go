package gocrud

import (
	"fmt"
	"reflect"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

type deleteByIDQueryBuilder interface {
	DeleteByIDQuery() (string, error)
}

type deleteReturningIDQueryBuilder interface {
	DeleteReturningIDQuery() (string, error)
}

type selectQueryBuilder interface {
	SelectQuery(order []string, limit int, offset int, filters sqlfilters.Filter) (string, error)
}

type selectCountQueryBuilder interface {
	SelectCountQuery(filters sqlfilters.Filter) (string, error)
}

type selectByIDQueryBuilder interface {
	SelectByIDQuery() (string, error)
}

type updateByIDQueryBuilder interface {
	UpdateByIDQuery() (string, error)
}

type insertOnConflictUpdateQueryBuilder interface {
	InsertOnConflictUpdateQuery() (string, error)
}

type insertQueryBuilder interface {
	InsertQuery() (string, error)
}

type updateQueryBuilder interface {
	UpdateQuery(values map[string]interface{}, filters sqlfilters.Filter) (string, error)
}

type createTableQueryBuilder interface {
	CreateTableQuery() (string, error)
}

type dropTableQueryBuilder interface {
	DropTableQuery() (string, error)
}

var queryMethodInterfaces = map[string]reflect.Type{
	"DeleteByIDQuery":             reflect.TypeOf((*deleteByIDQueryBuilder)(nil)).Elem(),
	"DeleteReturningIDQuery":      reflect.TypeOf((*deleteReturningIDQueryBuilder)(nil)).Elem(),
	"SelectQuery":                 reflect.TypeOf((*selectQueryBuilder)(nil)).Elem(),
	"SelectCountQuery":            reflect.TypeOf((*selectCountQueryBuilder)(nil)).Elem(),
	"SelectByIDQuery":             reflect.TypeOf((*selectByIDQueryBuilder)(nil)).Elem(),
	"UpdateByIDQuery":             reflect.TypeOf((*updateByIDQueryBuilder)(nil)).Elem(),
	"InsertOnConflictUpdateQuery": reflect.TypeOf((*insertOnConflictUpdateQueryBuilder)(nil)).Elem(),
	"InsertQuery":                 reflect.TypeOf((*insertQueryBuilder)(nil)).Elem(),
	"UpdateQuery":                 reflect.TypeOf((*updateQueryBuilder)(nil)).Elem(),
	"CreateTableQuery":            reflect.TypeOf((*createTableQueryBuilder)(nil)).Elem(),
	"DropTableQuery":              reflect.TypeOf((*dropTableQueryBuilder)(nil)).Elem(),
}

// checkQueryMethods returns an error when obj has a query method with a signature other than the one gocrud calls,
// which would be ignored otherwise.
func checkQueryMethods(obj interface{}) error {
	objType := reflect.TypeOf(obj)
	for name, iface := range queryMethodInterfaces {
		if _, ok := objType.MethodByName(name); ok && !objType.Implements(iface) {
			return fmt.Errorf("%w: %s.%s is %s, want %s", QueryMethodError, objType, name,
				reflect.ValueOf(obj).MethodByName(name).Type(), iface.Method(0).Type)
		}
	}

	return nil
}
