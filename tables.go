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

// TableName returns the name of the database table of obj.
func (c *CRUD) TableName(obj interface{}) (string, error) {
	builder, err := c.builder(obj)
	if err != nil {
		return "", getBuilderObjectCRUDError(err)
	}

	return builder.TableName(), nil
}

// ReferencedTables returns the names of the tables that the foreign keys of obj reference, without duplicates.
func (c *CRUD) ReferencedTables(obj interface{}) ([]string, error) {
	builder, err := c.builder(obj)
	if err != nil {
		return nil, getBuilderObjectCRUDError(err)
	}

	var tables []string
	seen := map[string]bool{}
	for _, column := range builder.Columns() {
		if column.ForeignKey != nil && !seen[column.ForeignKey.Table] {
			seen[column.ForeignKey.Table] = true
			tables = append(tables, column.ForeignKey.Table)
		}
	}

	return tables, nil
}
