package gocrud

import (
	"time"
)

// TimeValue returns t in the form that the database expects in the arguments of a query. gocrud converts times
// itself, so it is needed only for queries run directly on the database, e.g. with Tx().ExecContext.
func (c *CRUD) TimeValue(t time.Time) interface{} {
	return c.args([]interface{}{t})[0]
}

// args prepares the arguments of a query for the database. A database that stores times as text, SQLite, compares
// them as text, so they are converted to a format in which the texts are ordered the same way as the times.
func (c *CRUD) args(args []interface{}) []interface{} {
	format := c.sqlDialect.TimeFormat()
	if format == "" {
		return args
	}

	for i, arg := range args {
		switch v := arg.(type) {
		case time.Time:
			args[i] = v.UTC().Format(format)
		case *time.Time:
			if v != nil {
				args[i] = v.UTC().Format(format)
			}
		case **time.Time:
			// a nullable field
			if v != nil && *v != nil {
				args[i] = (*v).UTC().Format(format)
			}
		}
	}

	return args
}
