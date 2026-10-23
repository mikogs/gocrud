package gocrud

import (
	"context"
	"strings"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
)

// CreateTable creates a database table for the specified object, along with any
// indexes declared via 'idx'/'uidx' struct tags.
func (c *CRUD) CreateTable(ctx context.Context, obj interface{}) error {
	builder, err := c.builder(obj)
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}

	statements, err := createTableStatements(obj, builder)
	if err != nil {
		return err
	}

	for _, statement := range statements {
		if _, err := c.conn.ExecContext(ctx, statement); err != nil {
			return getDBFuncCRUDError("exec", err)
		}
	}

	return nil
}

// createTableStatements returns the statements that create the table of obj and its indexes. The query may consist
// of multiple ';'-separated statements, so they are returned separately rather than relying on driver support for
// multi-statement Exec calls.
func createTableStatements(obj interface{}, builder *sqlbuilder.Builder) ([]string, error) {
	query := builder.CreateTable()

	// if the object has a CreateTable method, use it.
	if createTableerImpl, ok := obj.(createTableQueryBuilder); ok {
		var err error
		query, err = createTableerImpl.CreateTableQuery()
		if err != nil {
			return nil, getObjFuncCRUDError("create table", err)
		}
	}

	var statements []string
	for _, statement := range strings.Split(query, ";") {
		if statement = strings.TrimSpace(statement); statement != "" {
			statements = append(statements, statement)
		}
	}

	return statements, nil
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
