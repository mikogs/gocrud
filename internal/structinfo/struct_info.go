package structinfo

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

var (
	ErrNotStructPointer = errors.New("obj must be a non-nil pointer to a struct")
	ErrIDField          = errors.New(`first field must be "ID" of type uint64 or string`)
	ErrNoColumns        = errors.New(`struct must have at least one field other than "ID"`)
	ErrPasswordField    = errors.New(`field tagged with "pass" must be of type string`)
	// ErrModificationFields is returned for a struct whose CreatedBy and ModifiedBy fields are of different types.
	ErrModificationFields = errors.New(`fields "CreatedBy" and "ModifiedBy" must be of the same type, uint64 or string`)
	// ErrColumnType is returned for a field with an invalid type tag, or of a Valuer type whose column type is not
	// known, which needs a type tag.
	ErrColumnType = errors.New("invalid column type")
	// ErrVersionField is returned for a struct with more than one field tagged with "version", or with one that is
	// not an integer, and for a value of the version field in UpdateMultiple, as gocrud sets it.
	ErrVersionField = errors.New(`field tagged with "version" must be the only one, of an integer type, and is set by gocrud`)
)

var (
	regexpColumnNameTag = regexp.MustCompile(`^([a-z0-9_]+\.)?[a-z0-9_]+$`)
	regexpIndexNameTag  = regexp.MustCompile(`^[a-z0-9_]+$`)
	regexpColumnTypeTag = regexp.MustCompile(`^[A-Z][A-Z0-9_]*(\([0-9]+(,[0-9]+)?\))?$`)
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
	// all ColumnFieldNames except ID, the version field, and CreatedAt and CreatedBy when the struct has the
	// modification fields.
	UpdateFieldNames []string
	// VersionField is the name of the field tagged with "version", whose value is checked and incremented on each
	// update, or empty when the struct has none.
	VersionField string
	// UniqueFields contains a list of field names that are marked with a tag 'uniq'.
	UniqueFields []string
	// PasswordFields contains a list of field names that are marked with a tag 'pass'.
	PasswordFields []string
	// TimeFields contains a list of names of the time.Time fields that are stored in the database.
	TimeFields []string
	// JSONFields contains the names of the fields that are stored as JSON, marked with a tag 'json'.
	JSONFields []string
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
func New(obj any, tagName string) (*StructInfo, error) {
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
		tag := field.Tag.Get(tagName)
		isJSON := slices.Contains(strings.Fields(tag), "json")

		if _, isRelation := relationTagKey(tag); isRelation {
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

		if !field.IsExported() || !isJSON && !IsFieldTypeSupported(field.Type) {
			continue
		}

		structInfo.FieldNames = append(structInfo.FieldNames, field.Name)
		structInfo.Fields[field.Name] = &FieldInfo{
			Nullable: IsNullableType(field.Type),
		}
		if isJSON {
			structInfo.Fields[field.Name].Nullable = field.Type.Kind() == reflect.Pointer
		}

		structInfo.setFieldFromTag(tag, field.Name)
		if err := structInfo.checkColumnType(field); err != nil {
			return nil, fmt.Errorf("struct %s: %w", objType.Name(), err)
		}

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
	if s.Fields["ID"].Version {
		return fmt.Errorf("%w: ID", ErrVersionField)
	}

	s.ColumnFieldNames = columns

	for _, column := range columns[1:] {
		if s.Fields[column].Version {
			if err := s.setVersionField(objType, column); err != nil {
				return err
			}
			continue
		}
		if s.ModificationFields && (column == "CreatedAt" || column == "CreatedBy") {
			continue
		}
		s.UpdateFieldNames = append(s.UpdateFieldNames, column)
	}

	for _, column := range columns {
		field, _ := objType.FieldByName(column)
		if s.Fields[column].JSON {
			s.JSONFields = append(s.JSONFields, column)
		} else if IsTimeType(BaseType(field.Type)) {
			s.TimeFields = append(s.TimeFields, column)
		}

		if s.Fields[column].Password && field.Type.Kind() != reflect.String {
			return fmt.Errorf("%w: %s", ErrPasswordField, column)
		}
	}

	return nil
}

func (s *StructInfo) setVersionField(objType reflect.Type, name string) error {
	field, _ := objType.FieldByName(name)
	info := s.Fields[name]
	if s.VersionField != "" || info.JSON || info.Password {
		return fmt.Errorf("%w: %s", ErrVersionField, name)
	}
	switch field.Type.Kind() {
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
	default:
		return fmt.Errorf("%w: %s", ErrVersionField, name)
	}
	s.VersionField = name
	return nil
}

// checkColumnType checks that the type of the column of a field is known: from its type tag, which is a name with an
// optional length, or precision and scale, e.g. NUMERIC(12,2), or from the type of the field. A field stored as JSON
// must be of a type that encoding/json writes.
func (s *StructInfo) checkColumnType(field reflect.StructField) error {
	info := s.Fields[field.Name]
	if info.Ignored {
		return nil
	}
	if info.JSON {
		if field.Name == "ID" || info.Password {
			return fmt.Errorf("%w: field %s: the ID and passwords cannot be stored as JSON", ErrColumnType, field.Name)
		}
		if IsValuerType(BaseType(field.Type)) {
			return fmt.Errorf("%w: field %s: %s is written with its Value method, not as JSON", ErrColumnType, field.Name, BaseType(field.Type))
		}
		if _, err := JSONZeroValue(field.Type); err != nil {
			return fmt.Errorf("%w: field %s: %w", ErrColumnType, field.Name, err)
		}
	}
	if info.OverrideColumnType != "" {
		if !regexpColumnTypeTag.MatchString(info.OverrideColumnType) {
			return fmt.Errorf("%w: field %s: %q", ErrColumnType, field.Name, info.OverrideColumnType)
		}
		return nil
	}
	if info.JSON {
		return nil
	}

	if ColumnGoType(field.Type) == "" {
		if _, err := ZeroValue(BaseType(field.Type)); err != nil {
			return fmt.Errorf("%w: field %s: %w", ErrColumnType, field.Name, err)
		}
		return fmt.Errorf("%w: field %s: the zero value of %s is NULL, so the column needs a type tag, e.g. type:TEXT",
			ErrColumnType, field.Name, BaseType(field.Type))
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

	if opt == "json" {
		s.Fields[fieldName].JSON = true
		return
	}

	if opt == "version" {
		s.Fields[fieldName].Version = true
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
		// checked by checkColumnType, as it is a part of the SQL of the queries
		s.Fields[fieldName].OverrideColumnType = strings.ToUpper(strings.TrimPrefix(opt, "type:"))
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
