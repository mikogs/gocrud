package postgres

// columnDefinitionFromField maps a struct field type to the definition of its database column.
// Nullable columns do not have NOT NULL and DEFAULT, so that their values can be NULL.
// A string foreign key holds a UUID, and has no default, as an empty string is not a UUID.
func columnDefinitionFromField(fieldName string, fieldType string, isUnique bool, overrideColumnType string, isNullable bool, isUUID bool) string {
	// 'ID' is a special field, a string ID holds a UUID
	if fieldName == "ID" {
		if fieldType == "string" {
			return "UUID PRIMARY KEY"
		}
		return "SERIAL PRIMARY KEY"
	}

	columnType, defaultValue := columnTypeFromField(fieldType)
	if overrideColumnType != "" {
		columnType = overrideColumnType
		defaultValue = "''"
	}

	definition := columnType
	switch {
	case isUUID:
		definition = "UUID"
		if !isNullable {
			definition += " NOT NULL"
		}
	case !isNullable:
		definition += " NOT NULL DEFAULT " + defaultValue
	}

	if isUnique {
		definition += " UNIQUE"
	}

	return definition
}

// columnTypeFromField returns the column type and the default value for a field type.
func columnTypeFromField(fieldType string) (string, string) {
	switch fieldType {
	case "bool":
		return "BOOLEAN", "false"
	case "int64", "int", "uint64", "uint":
		return "BIGINT", "0"
	case "int32", "uint32":
		return "INTEGER", "0"
	case "int16", "int8", "uint16", "uint8":
		return "SMALLINT", "0"
	case "float64":
		return "DOUBLE PRECISION", "0"
	case "float32":
		return "REAL", "0"
	case "time.Time":
		// the default is the zero value of time.Time
		return "TIMESTAMPTZ", "'0001-01-01 00:00:00+00'"
	default:
		// TODO: Consider something different
		return "VARCHAR(255)", "''"
	}
}

// columnType returns the type of the column for a field type, or the type set in the field tag.
func columnType(fieldType string, overrideColumnType string) string {
	if overrideColumnType != "" {
		return overrideColumnType
	}

	columnType, _ := columnTypeFromField(fieldType)
	return columnType
}
