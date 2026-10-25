package gocrud

import (
	"database/sql"
	"reflect"
	"regexp"
	"sync"

	"github.com/mikogs/gocrud/internal/dialect"
	"github.com/mikogs/gocrud/internal/sqlbuilder"
)

type CRUD struct {
	db *sql.DB
	// tx is set when the CRUD runs its queries in a transaction, see WithTx and RunInTx.
	tx *sql.Tx
	// conn is the tx when it is set, or the db otherwise.
	conn            DBTX
	builders        *builderCache
	tagName         string
	tableNamePrefix string
	dialect         string
	sqlDialect      dialect.Dialect
	getCountOnUniq  bool
	hook            QueryHook
}

// builderCache is shared between a CRUD and its copies running in transactions.
type builderCache struct {
	mu       sync.RWMutex
	builders map[reflect.Type]*sqlbuilder.Builder
}

// regexpTableNamePrefix matches the prefixes of table names, which are a part of the SQL of the queries, as the names
// of tables generated from the names of structs and the tags, which are checked in the same way.
var regexpTableNamePrefix = regexp.MustCompile(`^[a-z0-9_]*$`)

func New(db *sql.DB, options Options) *CRUD {
	if options.Dialect != DialectPostgres && options.Dialect != DialectSQLite {
		panic("gocrud: Options.Dialect must be set to DialectPostgres or DialectSQLite")
	}
	if !regexpTableNamePrefix.MatchString(options.TableNamePrefix) {
		panic("gocrud: Options.TableNamePrefix can have only lowercase letters, digits and underscores")
	}

	crud := &CRUD{
		db:   db,
		conn: db,
		builders: &builderCache{
			builders: make(map[reflect.Type]*sqlbuilder.Builder),
		},
	}

	crud.tagName = "crud"
	if options.TagName != "" {
		crud.tagName = options.TagName
	}

	if options.TableNamePrefix != "" {
		crud.tableNamePrefix = options.TableNamePrefix
	}

	crud.dialect = options.Dialect
	crud.sqlDialect = dialect.For(options.Dialect)
	crud.getCountOnUniq = options.GetCountOnUniq
	crud.hook = options.QueryHook
	crud.conn = crud.hooked(db)

	return crud
}

// PasswordFieldsFor returns the names of the password fields of obj.
func (c *CRUD) PasswordFieldsFor(obj any) []string {
	builder, err := c.builder(obj)
	if err != nil {
		return nil
	}
	return builder.PasswordFields()
}

// RelatedType returns the type of the related struct of the relation field of obj, e.g. Customer for the field
// Customer of an order, or Order for the field Orders of a customer. It returns false when field is not a relation
// field.
func (c *CRUD) RelatedType(obj any, field string) (reflect.Type, bool) {
	builder, err := c.builder(obj)
	if err != nil {
		return nil, false
	}

	relation := findRelation(builder, field)
	if relation == nil {
		return nil, false
	}
	return relation.Type, true
}
