package structinfo

type FieldInfo struct {
	Name string
	// OverrideColumnName is the name of the column in the database table.
	OverrideColumnName string
	// OverrideColumnType is the type of the column in the database table.
	OverrideColumnType string
	// Unique is set to true if the field is marked with a tag 'uniq'.
	Unique bool
	// Password is set to true if the field is marked with a 'pass' tag.
	Password bool
	// Nullable is set to true if the field is stored in a column that can be NULL: a pointer, or a Valuer type whose
	// zero value is NULL.
	Nullable bool
	// JSON is set to true if the field is marked with a 'json' tag, and is stored as JSON, with encoding/json.
	JSON bool
	// Version is set to true if the field is marked with a 'version' tag.
	Version bool
	// ForeignKeyTable is the name of the table that the field references with an 'fk:<table>' tag, without
	// the table name prefix.
	ForeignKeyTable string
	// UUID is set to true for a foreign key of type string or *string, which holds the UUID of the referenced row.
	UUID bool
	// OnDelete is the action set with an 'on_delete:<action>' tag: OnDeleteCascade, OnDeleteRestrict or
	// OnDeleteSetNull. It is empty when the tag is not set.
	OnDelete string
	// Ignored is set to true if the field is marked with a "-" tag.
	Ignored bool
	// Index is set to true if the field is marked with an 'idx' or 'uidx' tag.
	Index bool
	// IndexName is the name given via 'idx:<name>' or 'uidx:<name>'. Fields sharing the same
	// IndexName are combined into a single composite index. Empty when the tag has no name,
	// in which case the field gets its own auto-named, single-column index.
	IndexName string
	// IndexUnique is set to true if the field's index tag is 'uidx' (or 'uidx:<name>').
	IndexUnique bool
}
