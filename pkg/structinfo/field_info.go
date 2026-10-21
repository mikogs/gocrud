package structinfo

type FieldInfo struct {
	// Name contains name of the field.
	Name string
	// OverrideColumnName is the name of the column in the database table.
	OverrideColumnName string
	// OverrideColumnType is the type of the column in the database table.
	OverrideColumnType string
	// Default contains default value for the field.
	Default string
	// Unique is set to true if the field is marked with a tag 'uniq'.
	Unique bool
	// Password is set to true if the field is marked with a "password" tag.
	Password bool
	// Nullable is set to true if the field is a pointer, stored in a column that can be NULL.
	Nullable bool
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
