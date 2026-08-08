package structinfo

import (
	"reflect"
	"regexp"
	"strings"
)

var (
	regexpColumnNameTag  = regexp.MustCompile(`^(.[a-z0-9_]){0,1}[a-z0-9_]+$`)
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
	// UniqueFields contains a list of field names that are marked with a tag 'uniq'.
	UniqueFields []string
	// PasswordFields contains a list of field names that are marked with a tag 'pass'.
	PasswordFields []string
	// Indexes contains the database indexes built from fields marked with 'idx' or 'uidx' tags.
	Indexes []*IndexInfo
}

func New(obj interface{}, tagName string) *StructInfo {
	structInfo := &StructInfo{}

	objValue := reflect.ValueOf(obj)
	objIndirectValue := reflect.Indirect(objValue)
	objType := objIndirectValue.Type()

	numField := objType.NumField()

	structInfo.Fields = make(map[string]*FieldInfo, numField)
	structInfo.FieldNames = make([]string, 0, numField)
	structInfo.UniqueFields = make([]string, 0, numField)
	structInfo.PasswordFields = make([]string, 0, numField)

	modificationFields := 0

	for j := 0; j < objType.NumField(); j++ {
		field := objType.Field(j)
		fieldTypeKind := field.Type.Kind()

		// Only basic golang types are included as columns for the database table.
		// Check the function below for the details.
		if !IsFieldKindSupported(fieldTypeKind) {
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

	structInfo.buildIndexes()

	return structInfo
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
		if regexpColumnNameTag.MatchString(name) {
			s.Fields[fieldName].IndexName = name
		}

		return
	}

	if strings.HasPrefix(opt, "uidx:") {
		s.Fields[fieldName].Index = true
		s.Fields[fieldName].IndexUnique = true

		name := strings.TrimPrefix(opt, "uidx:")
		if regexpColumnNameTag.MatchString(name) {
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
