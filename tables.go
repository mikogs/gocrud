package gocrud

import (
	"context"
	"strings"
)

// CreateTable creates a database table for the specified object, along with any
// indexes declared via 'idx'/'uidx' struct tags.
func (c *CRUD) CreateTable(ctx context.Context, obj interface{}) error {
	builder, err := c.builder(obj)
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}

	var query string

	// if the object has a CreateTable method, use it.
	if createTableerImpl, ok := obj.(createTableQueryBuilder); ok {
		query, err = createTableerImpl.CreateTableQuery()
		if err != nil {
			return getObjFuncCRUDError("create table", err)
		}
	} else {
		query = builder.CreateTable()
	}

	// The query may consist of multiple ';'-separated statements (the table itself
	// plus any indexes), so each is executed separately rather than relying on
	// driver support for multi-statement Exec calls.
	for _, stmt := range strings.Split(query, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		_, err = c.conn.ExecContext(ctx, stmt)
		if err != nil {
			return getDBFuncCRUDError("exec", err)
		}
	}

	return nil
}

// DropTable drops the database table for the specified object.
func (c *CRUD) DropTable(ctx context.Context, obj interface{}) error {
	builder, err := c.builder(obj)
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}

	var query string
	// if the object has a DropTable method, use it.
	if dropTableerImpl, ok := obj.(dropTableQueryBuilder); ok {
		query, err = dropTableerImpl.DropTableQuery()
		if err != nil {
			return getObjFuncCRUDError("drop table", err)
		}
	} else {
		query = builder.DropTable()
	}

	_, err = c.conn.ExecContext(ctx, query)
	if err != nil {
		return getDBFuncCRUDError("exec", err)
	}

	return nil
}
