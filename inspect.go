package gocrud

import (
	"context"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
)

// The functions of this file read the tables of the database, as they are, for PlanMigration and CheckSchema.

// databaseColumn describes a column of an existing table.
type databaseColumn struct {
	name     string
	dataType string
	length   int
	nullable bool
}

// databaseColumns returns the columns of the table, or no columns when the table does not exist.
func (c *CRUD) databaseColumns(ctx context.Context, table string) (map[string]databaseColumn, error) {
	query := c.sqlDialect.ColumnsQuery()

	rows, err := c.conn.QueryContext(ctx, query, table)
	if err != nil {
		return nil, getDBFuncCRUDError("query table columns", err)
	}
	defer func() { _ = rows.Close() }()

	columns := map[string]databaseColumn{}
	for rows.Next() {
		var column databaseColumn
		if err := rows.Scan(&column.name, &column.dataType, &column.length, &column.nullable); err != nil {
			return nil, getDBFuncCRUDError("scan table columns", err)
		}
		columns[column.name] = column
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("query table columns", err)
	}

	return columns, nil
}

// databaseUniqueColumns returns the names of the columns that are unique on their own: they have a unique index
// on the column only, which is also how UNIQUE constraints are implemented. Partial indexes are not included.
func (c *CRUD) databaseUniqueColumns(ctx context.Context, table string) (map[string]bool, error) {
	query := c.sqlDialect.UniqueColumnsQuery()

	rows, err := c.conn.QueryContext(ctx, query, table)
	if err != nil {
		return nil, getDBFuncCRUDError("query unique columns", err)
	}
	defer func() { _ = rows.Close() }()

	columns := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, getDBFuncCRUDError("scan unique columns", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("query unique columns", err)
	}

	return columns, nil
}

// databaseForeignKeys returns the foreign keys of single columns of the table, by the column name.
func (c *CRUD) databaseForeignKeys(ctx context.Context, table string) (map[string]*sqlbuilder.ForeignKey, error) {
	query := c.sqlDialect.ForeignKeysQuery()

	rows, err := c.conn.QueryContext(ctx, query, table)
	if err != nil {
		return nil, getDBFuncCRUDError("query foreign keys", err)
	}
	defer func() { _ = rows.Close() }()

	foreignKeys := map[string]*sqlbuilder.ForeignKey{}
	for rows.Next() {
		var column string
		foreignKey := &sqlbuilder.ForeignKey{}
		if err := rows.Scan(&column, &foreignKey.Table, &foreignKey.OnDelete); err != nil {
			return nil, getDBFuncCRUDError("scan foreign keys", err)
		}
		foreignKeys[column] = foreignKey
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("query foreign keys", err)
	}

	return foreignKeys, nil
}

func (c *CRUD) databaseIndexes(ctx context.Context, table string) (map[string]bool, error) {
	query := c.sqlDialect.IndexesQuery()

	rows, err := c.conn.QueryContext(ctx, query, table)
	if err != nil {
		return nil, getDBFuncCRUDError("query table indexes", err)
	}
	defer func() { _ = rows.Close() }()

	indexes := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, getDBFuncCRUDError("scan table indexes", err)
		}
		indexes[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, getDBFuncCRUDError("query table indexes", err)
	}

	return indexes, nil
}
