package structinfo

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ErrRelation is returned for a field with a 'rel' tag that does not declare a valid relation.
var ErrRelation = errors.New("invalid relation")

const relationTagPrefix = "rel:"

type RelationKind int

const (
	// BelongsTo is a relation of a pointer field, e.g. Customer *Customer, to the object whose ID is stored in
	// the key field of this struct, e.g. CustomerID.
	BelongsTo RelationKind = iota + 1
	// HasMany is a relation of a slice field, e.g. Items []OrderItem, to the objects that store the ID of this
	// object in their key field, e.g. OrderItem.OrderID.
	HasMany
)

// RelationInfo describes a field that holds related objects, declared with a 'rel:<key field>' tag.
type RelationInfo struct {
	// Field is the name of the field that holds the related objects.
	Field string
	// Kind is BelongsTo for a pointer field, and HasMany for a slice field.
	Kind RelationKind
	// KeyField is the name of the field that holds the ID: in this struct for BelongsTo, and in the related
	// struct for HasMany. It is of the type of the ID that it holds, uint64 or string, or a pointer to it.
	KeyField string
	// Type is the type of the related struct.
	Type reflect.Type
	// PointerElements is set to true for a HasMany field that is a slice of pointers, e.g. []*OrderItem.
	PointerElements bool
}

// relationTagKey returns the key field of a 'rel' tag option, and false when the tag does not have it.
func relationTagKey(tag string) (string, bool) {
	for _, opt := range strings.Fields(tag) {
		if strings.HasPrefix(opt, relationTagPrefix) {
			return strings.TrimPrefix(opt, relationTagPrefix), true
		}
	}

	return "", false
}

// RelationFieldNames returns the names of the fields of the struct type t that have a 'rel' tag.
func RelationFieldNames(t reflect.Type, tagName string) []string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		if _, ok := relationTagKey(t.Field(i).Tag.Get(tagName)); ok {
			names = append(names, t.Field(i).Name)
		}
	}

	return names
}

// newRelationInfo checks the declaration of a relation field and returns its description. The key field of
// a BelongsTo relation is checked later, with the other fields of the struct.
func newRelationInfo(field reflect.StructField, tagName string) (*RelationInfo, error) {
	tag := field.Tag.Get(tagName)
	keyField, _ := relationTagKey(tag)

	relationError := func(reason string) error {
		return fmt.Errorf("%w: field %s: %s", ErrRelation, field.Name, reason)
	}

	if !field.IsExported() {
		return nil, relationError("the field must be exported")
	}
	if len(strings.Fields(tag)) != 1 {
		return nil, relationError("a relation field can have only the rel option")
	}
	if keyField == "" {
		return nil, relationError("the rel option must name the key field, e.g. rel:CustomerID")
	}

	relation := &RelationInfo{
		Field:    field.Name,
		KeyField: keyField,
	}

	switch {
	case field.Type.Kind() == reflect.Pointer && isRelatedStructType(field.Type.Elem()):
		relation.Kind = BelongsTo
		relation.Type = field.Type.Elem()
	case field.Type.Kind() == reflect.Slice && isRelatedStructType(field.Type.Elem()):
		relation.Kind = HasMany
		relation.Type = field.Type.Elem()
	case field.Type.Kind() == reflect.Slice && field.Type.Elem().Kind() == reflect.Pointer && isRelatedStructType(field.Type.Elem().Elem()):
		relation.Kind = HasMany
		relation.Type = field.Type.Elem().Elem()
		relation.PointerElements = true
	default:
		return nil, relationError("the field must be a pointer to a struct or a slice of structs, got " + field.Type.String())
	}

	idField, ok := relation.Type.FieldByName("ID")
	if !ok || !isIDType(idField.Type) {
		return nil, relationError(fmt.Sprintf(`the related struct %s must have an "ID" field of type uint64 or string`, relation.Type.Name()))
	}

	if relation.Kind == HasMany {
		key, ok := relation.Type.FieldByName(keyField)
		if !ok || !key.IsExported() || !isKeyType(key.Type) || strings.TrimSpace(key.Tag.Get(tagName)) == "-" {
			return nil, relationError(fmt.Sprintf(`the related struct %s must have a stored key field "%s" of type uint64, string, or a pointer to it`, relation.Type.Name(), keyField))
		}
	}

	return relation, nil
}

// checkRelationKeys checks that the key fields of BelongsTo relations are stored fields, and that the key fields of
// all the relations are of the type of the ID that they hold: the ID of the related struct for BelongsTo, and the ID
// of this struct for HasMany.
func (s *StructInfo) checkRelationKeys(objType reflect.Type) error {
	idType, _ := objType.FieldByName("ID")

	for _, relation := range s.Relations {
		keyType, heldIDType := reflect.Type(nil), idType.Type
		if relation.Kind == BelongsTo {
			key, ok := objType.FieldByName(relation.KeyField)
			fieldInfo := s.Fields[relation.KeyField]
			if !ok || fieldInfo == nil || fieldInfo.Ignored || !isKeyType(key.Type) {
				return fmt.Errorf(`%w: field %s: the struct must have a stored key field "%s" of type uint64, string, or a pointer to it`, ErrRelation, relation.Field, relation.KeyField)
			}
			keyType = key.Type
			relatedID, _ := relation.Type.FieldByName("ID")
			heldIDType = relatedID.Type
		} else {
			key, _ := relation.Type.FieldByName(relation.KeyField)
			keyType = key.Type
		}

		if BaseType(keyType).Kind() != heldIDType.Kind() {
			return fmt.Errorf(`%w: field %s: the key field "%s" is of type %s, and holds an ID of type %s`, ErrRelation, relation.Field, relation.KeyField, keyType, heldIDType)
		}
	}

	return nil
}

func isRelatedStructType(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && !IsTimeType(t)
}

func isIDType(t reflect.Type) bool {
	return t.Kind() == reflect.Uint64 || t.Kind() == reflect.String
}

// isKeyType checks if t can hold an ID: uint64 or string, or a pointer to it.
func isKeyType(t reflect.Type) bool {
	return isIDType(BaseType(t)) && IsFieldTypeSupported(t)
}
