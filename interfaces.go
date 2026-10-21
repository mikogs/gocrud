package gocrud

import (
	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

type deleteByIDQueryBuilder interface {
	DeleteByIDQuery() (string, error)
}

type deleteReturningIDQueryBuilder interface {
	DeleteReturningIDQuery() (string, error)
}

type selectQueryBuilder interface {
	SelectQuery(order []string, limit int, offset int, filters *sqlfilters.Filters) (string, error)
}

type selectCountQueryBuilder interface {
	SelectCountQuery(filters *sqlfilters.Filters) (string, error)
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
	UpdateQuery(values map[string]interface{}, filters *sqlfilters.Filters) (string, error)
}

type createTableQueryBuilder interface {
	CreateTableQuery() (string, error)
}

type dropTableQueryBuilder interface {
	DropTableQuery() (string, error)
}
