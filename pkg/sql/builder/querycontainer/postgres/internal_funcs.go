package postgres

import "fmt"

// Mapping database column type to struct field type
func columnDefinitionFromField(fieldName string, fieldType string, isUnique bool, overrideColumnType string) string {
	// 'ID' is a special field
	if fieldName == "ID" {
		return "SERIAL PRIMARY KEY"
	}

	if overrideColumnType != "" {
		definition := fmt.Sprintf("%s NOT NULL DEFAULT ''", overrideColumnType)

		if isUnique {
			definition += " UNIQUE"
		}

		return definition
	}

	definition := ""
	switch fieldType {
	case "string":
		definition = "VARCHAR(255) NOT NULL DEFAULT ''"
	case "bool":
		definition = "BOOLEAN NOT NULL DEFAULT false"
	case "int64":
		definition = "BIGINT NOT NULL DEFAULT 0"
	case "int32":
		definition = "INTEGER NOT NULL DEFAULT 0"
	case "int16":
		definition = "SMALLINT NOT NULL DEFAULT 0"
	case "int8":
		definition = "SMALLINT NOT NULL DEFAULT 0"
	case "int":
		definition = "BIGINT NOT NULL DEFAULT 0"
	case "uint64":
		definition = "BIGINT NOT NULL DEFAULT 0"
	case "uint32":
		definition = "INTEGER NOT NULL DEFAULT 0"
	case "uint16":
		definition = "SMALLINT NOT NULL DEFAULT 0"
	case "uint8":
		definition = "SMALLINT NOT NULL DEFAULT 0"
	case "uint":
		definition = "BIGINT NOT NULL DEFAULT 0"
	case "time.Time":
		// the default is the zero value of time.Time
		definition = "TIMESTAMPTZ NOT NULL DEFAULT '0001-01-01 00:00:00+00'"
	// TODO: Consider something different
	default:
		definition = "VARCHAR(255) NOT NULL DEFAULT ''"
	}

	if isUnique {
		definition += " UNIQUE"
	}

	return definition
}
