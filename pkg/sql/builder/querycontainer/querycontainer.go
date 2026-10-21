package querycontainer

type DropTableBuilder interface {
	BuildDropTableQuery(tableNamePrefix string) (string, error)
}

type CreateTableBuilder interface {
	BuildCreateTableQuery(tableNamePrefix string) (string, error)
}

type DeleteByIDBuilder interface {
	BuildDeleteByIDQuery(tableNamePrefix string) (string, error)
}

type DeletePrefixBuilder interface {
	BuildDeletePrefixQuery(tableNamePrefix string) (string, error)
}

type UpdateByIDBuilder interface {
	BuildUpdateByIDQuery(tableNamePrefix string) (string, error)
}

type UpdatePrefixBuilder interface {
	BuildUpdatePrefixQuery(tableNamePrefix string) (string, error)
}

type InsertBuilder interface {
	BuildInsertQuery(tableNamePrefix string) (string, error)
}

type InsertOnConflictUpdateBuilder interface {
	BuildInsertOnConflictUpdateQuery(tableNamePrefix string) (string, error)
}

type SelectByIDBuilder interface {
	BuildSelectByIDQuery(tableNamePrefix string) (string, error)
}

type SelectPrefixBuilder interface {
	BuildSelectPrefixQuery(tableNamePrefix string) (string, error)
}

type SelectCountPrefixBuilder interface {
	BuildSelectCountPrefixQuery(tableNamePrefix string) (string, error)
}

// Column describes a database column of a struct field.
type Column struct {
	// Name is the name of the column, without an alias.
	Name string
	// Type is the type of the column, e.g. "VARCHAR(255)".
	Type string
	// Nullable is set to true if the column can be NULL.
	Nullable bool
	// Unique is set to true if the column is unique.
	Unique bool
	// IsID is set to true for the column of the ID field.
	IsID bool
	// AddStatements are the statements that add the column to an existing table.
	AddStatements []string
}
