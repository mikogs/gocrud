package structinfo

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
)

// ErrForeignKey is returned for a field with 'fk' or 'on_delete' tags that do not declare a valid foreign key.
var ErrForeignKey = errors.New("invalid foreign key")

// Actions of the 'on_delete' tag.
const (
	OnDeleteCascade  = "cascade"
	OnDeleteRestrict = "restrict"
	OnDeleteSetNull  = "set_null"
)

var regexpTableNameTag = regexp.MustCompile(`^[a-z0-9_]+$`)

// checkForeignKeys checks the fields with 'fk' and 'on_delete' tags: a foreign key is a field of type uint64, or
// *uint64 for a reference that is optional, that references the ID of another table. A foreign key that references
// a table with UUIDs is of type string or *string.
func (s *StructInfo) checkForeignKeys(objType reflect.Type) error {
	for _, fieldName := range s.ColumnFieldNames {
		fieldInfo := s.Fields[fieldName]
		if fieldInfo.ForeignKeyTable == "" && fieldInfo.OnDelete == "" {
			continue
		}

		foreignKeyError := func(reason string) error {
			return fmt.Errorf("%w: field %s: %s", ErrForeignKey, fieldName, reason)
		}

		field, _ := objType.FieldByName(fieldName)
		switch {
		case fieldInfo.ForeignKeyTable == "":
			return foreignKeyError("on_delete needs an fk option, e.g. fk:customer")
		case !regexpTableNameTag.MatchString(fieldInfo.ForeignKeyTable):
			return foreignKeyError(fmt.Sprintf(`"%s" is not a valid table name`, fieldInfo.ForeignKeyTable))
		case fieldName == "ID":
			return foreignKeyError("the ID field cannot be a foreign key")
		case !isKeyType(field.Type):
			return foreignKeyError("the field must be of type uint64, *uint64, string or *string, got " + field.Type.String())
		}

		if BaseType(field.Type).Kind() == reflect.String {
			fieldInfo.UUID = true
			s.UUIDFields = append(s.UUIDFields, fieldName)
		}

		switch fieldInfo.OnDelete {
		case "", OnDeleteCascade, OnDeleteRestrict:
		case OnDeleteSetNull:
			if !fieldInfo.Nullable {
				return foreignKeyError("on_delete:set_null needs a field of type *uint64 or *string, which can be NULL")
			}
		default:
			return foreignKeyError(fmt.Sprintf(`on_delete must be %s, %s or %s, got "%s"`, OnDeleteCascade, OnDeleteRestrict, OnDeleteSetNull, fieldInfo.OnDelete))
		}

		s.ForeignKeyFields = append(s.ForeignKeyFields, fieldName)
	}

	return nil
}
