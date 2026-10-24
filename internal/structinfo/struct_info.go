package structinfo

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var (
	ErrNotStructPointer = errors.New("obj must be a non-nil pointer to a struct")
	ErrIDField          = errors.New(`first field must be "ID" of type uint64 or string`)
	ErrNoColumns        = errors.New(`struct must have at least one field other than "ID"`)
	ErrPasswordField    = errors.New(`field tagged with "pass" must be of type string`)
	// ErrModificationFields is returned for a struct whose CreatedBy and ModifiedBy fields are of different types.
	ErrModificationFields = errors.New(`fields "CreatedBy" and "ModifiedBy" must be of the same type, uint64 or string`)
)

var (
	regexpColumnNameTag  = regexp.MustCompile(`^([a-z0-9_]+\.)?[a-z0-9_]+$`)
	regexpIndexNameTag   = regexp.MustCompile(`^[a-z0-9_]+$`)
	regexpVarCharDBTypes = regexp.MustCompile(`^(VARCHAR|CHARACTER VARYING|BPCHAR|CHAR|CHARACTER)\([0-9]+\)$`)
)

type StructInfo struct {
	Name string
	// UUID indicates that the ID field is a string with a UUID, instead of a uint64 assigned by the database.
	UUID bool
	// ModificationFields indicates whether the struct has the following fields: CreatedAt and ModifiedAt of type time.Time,
	// CreatedBy and ModifiedBy of type uint64, or of type string, e.g. for the IDs of users in another system.
	ModificationFields bool
	TableName          string
	// AliasedColumnNames set to true indicates that some column names are in a format of "alias.column_name".
	AliasedColumnNames bool
	Fields             map[string]*FieldInfo
	FieldNames         []string
	// ColumnFieldNames contains an ordered list of names of the fields that are stored in the database (not ignored).
	ColumnFieldNames []string
	// UpdateFieldNames contains an ordered list of names of the fields that are written when an object is updated:
	// all ColumnFieldNames except ID, and CreatedAt and CreatedBy when the struct has the modification fields.
	UpdateFieldNames []string
	// UniqueFields contains a list of field names that are marked with a tag 'uniq'.
	UniqueFields []string
	// PasswordFields contains a list of field names that are marked with a tag 'pass'.
	PasswordFields []string
	// TimeFields contains a list of names of the time.Time fields that are stored in the database.
	TimeFields []string
	// Indexes contains the database indexes built from fields marked with 'idx' or 'uidx' tags.
	Indexes []*IndexInfo
	// Relations contains the fields that hold related objects, declared with 'rel' tags, in the order of the fields.
	Relations []*RelationInfo
	// ForeignKeyFields contains the names of the fields that are foreign keys, declared with 'fk' tags.
	ForeignKeyFields []string
	// UUIDFields contains the names of the fields that hold UUIDs: the ID field when UUID is set, and the foreign
	// keys of type string or *string.
	UUIDFields []string
}

// New returns information on the obj struct fields, read from their tagName tags.
// The first field that is stored in the database must be "ID" of type uint64 or string.
func New(obj interface{}, tagName string) (*StructInfo, error) {
	structInfo := &StructInfo{}

	objValue := reflect.ValueOf(obj)
	if objValue.Kind() != reflect.Pointer || objValue.IsNil() || objValue.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w, got %T", ErrNotStructPointer, obj)
	}

	objType := objValue.Elem().Type()
	structInfo.Name = objType.Name()

	numField := objType.NumField()

	structInfo.Fields = make(map[string]*FieldInfo, numField)
	structInfo.FieldNames = make([]string, 0, numField)
	structInfo.UniqueFields = make([]string, 0, numField)
	structInfo.PasswordFields = make([]string, 0, numField)

	modificationFields := 0

	for j := 0; j < objType.NumField(); j++ {
		field := objType.Field(j)

		if _, isRelation := relationTagKey(field.Tag.Get(tagName)); isRelation {
			// a field stored in a column cannot hold related objects
			if field.IsExported() && IsFieldTypeSupported(field.Type) {
				return nil, fmt.Errorf("struct %s: %w: field %s: the rel option is for a field that holds related objects, e.g. Customer *Customer, not for a column", objType.Name(), ErrRelation, field.Name)
			}

			relation, err := newRelationInfo(field, tagName)
			if err != nil {
				return nil, fmt.Errorf("struct %s: %w", objType.Name(), err)
			}
			structInfo.Relations = append(structInfo.Relations, relation)
			continue
		}

		if !field.IsExported() || !IsFieldTypeSupported(field.Type) {
			continue
		}

		structInfo.FieldNames = append(structInfo.FieldNames, field.Name)
		structInfo.Fields[field.Name] = &FieldInfo{
			Nullable: IsNullableType(field.Type),
		}

		structInfo.setFieldFromTag(field.Tag.Get(tagName), field.Name)

		if IsFieldModification(field.Name, field.Type) {
			modificationFields++
		}
	}

	if modificationFields == 4 {
		structInfo.ModificationFields = true

		createdBy, _ := objType.FieldByName("CreatedBy")
		modifiedBy, _ := objType.FieldByName("ModifiedBy")
		if createdBy.Type.Kind() != modifiedBy.Type.Kind() {
			return nil, fmt.Errorf("struct %s: %w", objType.Name(), ErrModificationFields)
		}
	}

	err := structInfo.checkColumns(objType)
	if err != nil {
		return nil, fmt.Errorf("struct %s: %w", objType.Name(), err)
	}

	err = structInfo.checkRelationKeys(objType)
	if err != nil {
		return nil, fmt.Errorf("struct %s: %w", objType.Name(), err)
	}

	err = structInfo.checkForeignKeys(objType)
	if err != nil {
		return nil, fmt.Errorf("struct %s: %w", objType.Name(), err)
	}

	structInfo.buildIndexes()

	return structInfo, nil
}

// checkColumns checks if the fields stored in the database start with "ID" of type uint64 or string, and are
// followed by at least one other field, as the generated queries rely on it.
func (s *StructInfo) checkColumns(objType reflect.Type) error {
	columns := make([]string, 0, len(s.FieldNames))
	for _, fieldName := range s.FieldNames {
		if !s.Fields[fieldName].Ignored {
			columns = append(columns, fieldName)
		}
	}

	if len(columns) == 0 || columns[0] != "ID" {
		return ErrIDField
	}

	idField, _ := objType.FieldByName("ID")
	switch idField.Type.Kind() {
	case reflect.Uint64:
	case reflect.String:
		s.UUID = true
		s.UUIDFields = append(s.UUIDFields, "ID")
	default:
		return ErrIDField
	}

	if len(columns) == 1 {
		return ErrNoColumns
	}

	s.ColumnFieldNames = columns

	for _, column := range columns[1:] {
		if s.ModificationFields && (column == "CreatedAt" || column == "CreatedBy") {
			continue
		}
		s.UpdateFieldNames = append(s.UpdateFieldNames, column)
	}

	for _, column := range columns {
		field, _ := objType.FieldByName(column)
		if IsTimeType(BaseType(field.Type)) {
			s.TimeFields = append(s.TimeFields, column)
		}

		if s.Fields[column].Password && field.Type.Kind() != reflect.String {
			return fmt.Errorf("%w: %s", ErrPasswordField, column)
		}
	}

	return nil
}

func (s *StructInfo) setFieldFromTag(tag string, fieldName string) {
	tag = strings.TrimSpace(tag)

	if tag == "-" && fieldName != "ID" {
		s.Fields[fieldName].Ignored = true
		return
	}

	opts := strings.Split(tag, " ")
	for _, opt := range opts {
		s.setFieldFromTagOptWithoutVal(opt, fieldName)
	}
}

func (s *StructInfo) setFieldFromTagOptWithoutVal(opt string, fieldName string) {
	if opt == "uniq" {
		s.Fields[fieldName].Unique = true
		s.UniqueFields = append(s.UniqueFields, fieldName)
		return
	}

	if opt == "pass" {
		s.Fields[fieldName].Password = true
		s.PasswordFields = append(s.PasswordFields, fieldName)
		return
	}

	if opt == "idx" {
		s.Fields[fieldName].Index = true
		return
	}

	if opt == "uidx" {
		s.Fields[fieldName].Index = true
		s.Fields[fieldName].IndexUnique = true
		return
	}

	if strings.HasPrefix(opt, "idx:") {
		s.Fields[fieldName].Index = true

		name := strings.TrimPrefix(opt, "idx:")
		if regexpIndexNameTag.MatchString(name) {
			s.Fields[fieldName].IndexName = name
		}

		return
	}

	if strings.HasPrefix(opt, "uidx:") {
		s.Fields[fieldName].Index = true
		s.Fields[fieldName].IndexUnique = true

		name := strings.TrimPrefix(opt, "uidx:")
		if regexpIndexNameTag.MatchString(name) {
			s.Fields[fieldName].IndexName = name
		}

		return
	}

	if strings.HasPrefix(opt, "type:") {
		typeArr := strings.Split(opt, ":")
		typeUpperCase := strings.ToUpper(typeArr[1])
		if typeUpperCase == "TEXT" || typeUpperCase == "BPCHAR" {
			s.Fields[fieldName].OverrideColumnType = typeUpperCase
		} else {
			if regexpVarCharDBTypes.MatchString(typeUpperCase) {
				s.Fields[fieldName].OverrideColumnType = typeUpperCase
			}
		}

		return
	}

	if strings.HasPrefix(opt, "fk:") {
		s.Fields[fieldName].ForeignKeyTable = strings.TrimPrefix(opt, "fk:")
		return
	}

	if strings.HasPrefix(opt, "on_delete:") {
		s.Fields[fieldName].OnDelete = strings.TrimPrefix(opt, "on_delete:")
		return
	}

	if strings.HasPrefix(opt, "name:") {
		nameArr := strings.Split(opt, ":")
		name := nameArr[1]

		if regexpColumnNameTag.MatchString(name) {
			s.Fields[fieldName].OverrideColumnName = name

			if strings.Contains(name, ".") {
				s.AliasedColumnNames = true
			}
		}

		return
	}
}
