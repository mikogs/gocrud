package sqlite

import (
	"fmt"
	"time"
)

func columnDefinitionFromField(fieldName string, fieldType string, isUnique bool, overrideColumnType string) string {
	if fieldName == "ID" {
		return "INTEGER PRIMARY KEY"
	}

	if overrideColumnType != "" {
		definition := fmt.Sprintf("%s NOT NULL DEFAULT ''", overrideColumnType)
		if isUnique {
			definition += " UNIQUE"
		}
		return definition
	}

	var definition string
	switch fieldType {
	case "string":
		definition = "TEXT NOT NULL DEFAULT ''"
	case "bool":
		definition = "INTEGER NOT NULL DEFAULT 0"
	case "float32", "float64":
		definition = "REAL NOT NULL DEFAULT 0"
	case "time.Time":
		// the driver reads DATETIME columns as time.Time, the default is the zero value of time.Time
		definition = "DATETIME NOT NULL DEFAULT '" + TimeValue(time.Time{}) + "'"
	default:
		// all int/uint variants
		definition = "INTEGER NOT NULL DEFAULT 0"
	}

	if isUnique {
		definition += " UNIQUE"
	}

	return definition
}
