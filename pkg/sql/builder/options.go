package builder

import "reflect"

const (
	DialectPostgres = "postgres"
	DialectSQLite   = "sqlite"
)

// Options are passed to Builder to change default values.
type Options struct {
	TableNamePrefix string
	StructName      string
	TagName         string
	Dialect         string
	// RelatedBuilder returns the Builder of a related struct type, for conditions on the fields of related structs,
	// e.g. Customer.Country. When it is nil, such conditions cannot be used.
	RelatedBuilder func(t reflect.Type) (*Builder, error)
}
