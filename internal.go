package gocrud

import (
	"reflect"
	"time"

	sqlbuilder "github.com/mikogs/gocrud/pkg/sql/builder"
	sqliteQueryContainer "github.com/mikogs/gocrud/pkg/sql/builder/querycontainer/sqlite"
)

func (c *CRUD) builder(obj interface{}) (*sqlbuilder.Builder, error) {
	objType := reflect.TypeOf(obj)

	c.builders.mu.RLock()
	builder, ok := c.builders.builders[objType]
	c.builders.mu.RUnlock()
	if ok {
		return builder, nil
	}

	builder, err := sqlbuilder.New(obj, sqlbuilder.Options{
		TableNamePrefix: c.tableNamePrefix,
		TagName:         c.tagName,
		Dialect:         c.dialect,
	})
	if err != nil {
		return nil, err
	}

	if err := checkQueryMethods(obj); err != nil {
		return nil, err
	}

	c.builders.mu.Lock()
	c.builders.builders[objType] = builder
	c.builders.mu.Unlock()

	return builder, nil
}

// TimeValue returns t in the form that the database expects in the arguments of a query. gocrud converts times
// itself, so it is needed only for queries run directly on the database, e.g. with Tx().ExecContext.
func (c *CRUD) TimeValue(t time.Time) interface{} {
	return c.args([]interface{}{t})[0]
}

// args prepares the arguments of a query for the database. SQLite stores times as strings, and compares them as
// strings, so they are converted to a format in which the strings are ordered the same way as the times.
func (c *CRUD) args(args []interface{}) []interface{} {
	if c.dialect != DialectSQLite {
		return args
	}

	for i, arg := range args {
		switch v := arg.(type) {
		case time.Time:
			args[i] = sqliteQueryContainer.TimeValue(v)
		case *time.Time:
			if v != nil {
				args[i] = sqliteQueryContainer.TimeValue(*v)
			}
		case **time.Time:
			// a nullable field
			if v != nil && *v != nil {
				args[i] = sqliteQueryContainer.TimeValue(**v)
			}
		}
	}

	return args
}
