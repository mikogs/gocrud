package structinfo

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var (
	NotStructPointerError = errors.New("obj must be a non-nil pointer to a struct")
	IDFieldError          = errors.New(`first field must be "ID" of type uint64`)
	NoColumnsError        = errors.New(`struct must have at least one field other than "ID"`)
)

var (
	regexpColumnNameTag  = regexp.MustCompile(`^([a-z0-9_]+\.)?[a-z0-9_]+$`)
	regexpIndexNameTag   = regexp.MustCompile(`^[a-z0-9_]+$`)
	regexpVarCharDBTypes = regexp.MustCompile(`^(VARCHAR|CHARACTER VARYING|BPCHAR|CHAR|CHARACTER)\([0-9]+\)$`)
)

type StructInfo struct {
	// Name of the struct.
	Name string
	// ModificationFields indicates whether the struct has the following fields: CreatedAt, CreatedBy, ModifiedAt, ModifiedBy.
	ModificationFields bool
	// TableName is the name of the table in the database.
	TableName string
	// AliasedColumnNames set to true indicates that some column names are in a format of "alias.column_name".
	AliasedColumnNames bool
	// Fields contains info on fields.
	Fields map[string]*FieldInfo
	// FieldNames contains an ordered list of field names.
	FieldNames []string
	// ColumnFieldNames contains an ordered list of names of the fields that are stored in the database (not ignored).
	ColumnFieldNames []string
	// UniqueFields contains a list of field names that are marked with a tag 'uniq'.
	UniqueFields []string
	// PasswordFields contains a list of field names that are marked with a tag 'pass'.
	PasswordFields []string
	// TimeFields contains a list of names of the time.Time fields that are stored in the database.
	TimeFields []string
	// Indexes contains the database indexes built from fields marked with 'idx' or 'uidx' tags.
	Indexes []*IndexInfo
}

// New returns information on the obj struct fields, read from their tagName tags.
// The first field that is stored in the database must be "ID" of type uint64.
func New(obj interface{}, tagName string) (*StructInfo, error) {
	structInfo := &StructInfo{}

	objValue := reflect.ValueOf(obj)
	if objValue.Kind() != reflect.Pointer || objValue.IsNil() || objValue.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w, got %T", NotStructPointerError, obj)
	}

	objType := objValue.Elem().Type()

	numField := objType.NumField()

	structInfo.Fields = make(map[string]*FieldInfo, numField)
	structInfo.FieldNames = make([]string, 0, numField)
	structInfo.UniqueFields = make([]string, 0, numField)
	structInfo.PasswordFields = make([]string, 0, numField)

	modificationFields := 0

	for j := 0; j < objType.NumField(); j++ {
		field := objType.Field(j)
		fieldTypeKind := field.Type.Kind()

		// Only exported fields of basic golang types are included as columns for the database table.
		// Check the function below for the details.
		if !field.IsExported() || !IsFieldTypeSupported(field.Type) {
			continue
		}

		structInfo.FieldNames = append(structInfo.FieldNames, field.Name)
		structInfo.Fields[field.Name] = &FieldInfo{}

		// Get value of a field's 'sql' and 'sql_val' tags (unless different when TagName provided in options).
		tagValue := field.Tag.Get(tagName)
		valTagValue := field.Tag.Get(tagName + "_val")

		// Go through tag values and parse out the ones we're interested in.
		structInfo.setFieldFromTag(tagValue, field.Name)

		if valTagValue != "" {
			structInfo.Fields[field.Name].Default = valTagValue
		}

		if IsFieldModification(field.Name, fieldTypeKind) {
			modificationFields++
		}
	}

	if modificationFields == 4 {
		structInfo.ModificationFields = true
	}

	err := structInfo.checkColumns(objType)
	if err != nil {
		return nil, fmt.Errorf("struct %s: %w", objType.Name(), err)
	}

	structInfo.buildIndexes()

	return structInfo, nil
}

// checkColumns checks if the fields stored in the database start with "ID" of type uint64 and are
// followed by at least one other field, as the generated queries rely on it.
func (s *StructInfo) checkColumns(objType reflect.Type) error {
	columns := make([]string, 0, len(s.FieldNames))
	for _, fieldName := range s.FieldNames {
		if !s.Fields[fieldName].Ignored {
			columns = append(columns, fieldName)
		}
	}

	if len(columns) == 0 || columns[0] != "ID" {
		return IDFieldError
	}

	idField, _ := objType.FieldByName("ID")
	if idField.Type.Kind() != reflect.Uint64 {
		return IDFieldError
	}

	if len(columns) == 1 {
		return NoColumnsError
	}

	s.ColumnFieldNames = columns

	for _, column := range columns {
		field, _ := objType.FieldByName(column)
		if IsTimeType(field.Type) {
			s.TimeFields = append(s.TimeFields, column)
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

	if strings.HasPrefix(opt, "name:") {
		nameArr := strings.Split(opt, ":")
		name := nameArr[1]

		if regexpColumnNameTag.MatchString(name) {
			s.Fields[fieldName].OverrideColumnName = name

			// If a column name is in the form of "alias.column_name", then we set the struct info's
			// AliasedColumnNames to true.
			if strings.Contains(name, ".") {
				s.AliasedColumnNames = true
			}
		}

		return
	}
}
