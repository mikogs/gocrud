package gocrud

import (
	"database/sql"
	"reflect"
	"sync"

	sqlbuilder "github.com/mikogs/gocrud/pkg/sql/builder"
)

type CRUD struct {
	db              *sql.DB
	buildersMu      sync.RWMutex
	builders        map[reflect.Type]*sqlbuilder.Builder
	tagName         string
	tableNamePrefix string
	dialect         string
	flags           uint64
}

func New(db *sql.DB, options Options) *CRUD {
	if options.Dialect != DialectPostgres && options.Dialect != DialectSQLite {
		panic("gocrud: Options.Dialect must be set to DialectPostgres or DialectSQLite")
	}

	crud := &CRUD{
		db:       db,
		builders: make(map[reflect.Type]*sqlbuilder.Builder),
	}

	crud.tagName = "crud"
	if options.TagName != "" {
		crud.tagName = options.TagName
	}

	if options.TableNamePrefix != "" {
		crud.tableNamePrefix = options.TableNamePrefix
	}

	crud.dialect = options.Dialect
	crud.flags = options.Flags

	return crud
}

func (c *CRUD) SetFlag(flag uint64) {
	if c.flags&flag == 0 {
		c.flags |= flag
	}
}

// PasswordFieldsFor returns the struct field names tagged as passwords for the
// type of obj. Returns nil if obj has no password fields.
func (c *CRUD) PasswordFieldsFor(obj interface{}) []string {
	builder, err := c.builder(obj)
	if err != nil {
		return nil
	}
	return builder.PasswordFields()
}
