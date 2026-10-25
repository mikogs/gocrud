package gocrud

type Options struct {
	TableNamePrefix string
	TagName         string
	Dialect         string
	// GetCountOnUniq makes Save count the rows with the values of the unique fields before it saves an object, and
	// return a UniqError for a value that another row has, also when the table has no UNIQUE constraint.
	GetCountOnUniq bool
	// QueryHook is called around each query, see QueryHook.
	QueryHook QueryHook
}

const (
	DialectPostgres = "postgres"
	DialectSQLite   = "sqlite"
)
