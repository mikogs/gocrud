package gocrud

type Options struct {
	TableNamePrefix string
	TagName         string
	Dialect         string
	Flags           uint64
	// QueryHook is called around each query, see QueryHook.
	QueryHook QueryHook
}

const (
	GetCountOnUniq = 1
)

const (
	DialectPostgres = "postgres"
	DialectSQLite   = "sqlite"
)
