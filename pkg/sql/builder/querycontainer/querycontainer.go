package querycontainer

import "fmt"

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
	// ForeignKey is the foreign key of the column, or nil when it does not reference another table.
	ForeignKey *ForeignKey
	// AddStatements are the statements that add the column to an existing table.
	AddStatements []string
}

// ForeignKey describes the foreign key of a column.
type ForeignKey struct {
	// Table is the name of the referenced table, with the table name prefix.
	Table string
	// OnDelete is the SQL action on delete, e.g. "CASCADE", or "NO ACTION" when it is not set.
	OnDelete string
}

// NewForeignKey returns the foreign key of a field with the 'fk:<table>' and 'on_delete:<action>' tags,
// or nil when table is empty.
func NewForeignKey(tableNamePrefix string, table string, onDelete string) *ForeignKey {
	if table == "" {
		return nil
	}

	foreignKey := &ForeignKey{Table: tableNamePrefix + table, OnDelete: "NO ACTION"}
	switch onDelete {
	case "cascade":
		foreignKey.OnDelete = "CASCADE"
	case "restrict":
		foreignKey.OnDelete = "RESTRICT"
	case "set_null":
		foreignKey.OnDelete = "SET NULL"
	}

	return foreignKey
}

// Clause returns the REFERENCES clause of a column definition, starting with a space, or an empty string for nil.
func (f *ForeignKey) Clause() string {
	if f == nil {
		return ""
	}

	clause := fmt.Sprintf(` REFERENCES "%s"("id")`, f.Table)
	if f.OnDelete != "NO ACTION" {
		clause += " ON DELETE " + f.OnDelete
	}

	return clause
}

// KeepEmptyPassword returns the value of a column in the SET clause of an update: the placeholder, or, for a password
// column, an expression that keeps the stored password when the value is empty, as the password fields of an object
// are empty after it is loaded. table and column are quoted.
func KeepEmptyPassword(placeholder string, table string, column string, password bool) string {
	if !password {
		return placeholder
	}
	return fmt.Sprintf("COALESCE(NULLIF(%s,''),%s.%s)", placeholder, table, column)
}
