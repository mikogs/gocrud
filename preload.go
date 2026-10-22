package gocrud

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
	sqlbuilder "github.com/mikogs/gocrud/pkg/sql/builder"
	"github.com/mikogs/gocrud/pkg/structinfo"
)

// ErrPreload is returned when a name in the Preload option is not a relation field of the struct.
var ErrPreload = errors.New("invalid preload")

// preloadChunkSize is the maximum number of keys in a query that loads related objects. Databases limit
// the number of parameters of a query.
var preloadChunkSize = 1000

// preloadTree is a set of preload paths, grouped by their first relation, e.g. "Items.Product" and "Items.Supplier"
// are the paths "Product" and "Supplier" of the relation "Items".
type preloadTree struct {
	relations []string
	paths     map[string][]string
}

// newPreloadTree groups the preload paths by their first relation, in the order they are first named.
func newPreloadTree(names []string, prefix string) (*preloadTree, error) {
	tree := &preloadTree{paths: map[string][]string{}}

	for _, name := range names {
		relation, path, isNested := strings.Cut(name, ".")
		if relation == "" || (isNested && path == "") {
			return nil, fmt.Errorf("%w: %s: the path has an empty relation name", ErrPreload, prefix+name)
		}

		if _, ok := tree.paths[relation]; !ok {
			tree.relations = append(tree.relations, relation)
			tree.paths[relation] = nil
		}
		if isNested {
			tree.paths[relation] = append(tree.paths[relation], path)
		}
	}

	return tree, nil
}

// checkPreload checks that the preload paths name relation fields, starting with the struct of builder.
func (c *CRUD) checkPreload(builder *sqlbuilder.Builder, names []string, prefix string) error {
	tree, err := newPreloadTree(names, prefix)
	if err != nil {
		return err
	}

	for _, name := range tree.relations {
		relation := findRelation(builder, name)
		if relation == nil {
			return fmt.Errorf("%w: %s: %s is not a relation field of %s", ErrPreload, prefix+name, name, builder.StructName())
		}

		if len(tree.paths[name]) == 0 {
			continue
		}

		relatedBuilder, err := c.builder(reflect.New(relation.Type).Interface())
		if err != nil {
			return err
		}
		if err := c.checkPreload(relatedBuilder, tree.paths[name], prefix+name+"."); err != nil {
			return err
		}
	}

	return nil
}

// preload loads the related objects of objs for the relation fields named in names, which are checked with
// checkPreload before. A name can be a path to the relations of related objects, e.g. "Items.Product", which
// loads the items and their products. Each relation is loaded with one query per preloadChunkSize objects.
func (c *CRUD) preload(ctx context.Context, builder *sqlbuilder.Builder, objs []interface{}, names []string) error {
	return c.preloadPaths(ctx, builder, objs, names)
}

func (c *CRUD) preloadPaths(ctx context.Context, builder *sqlbuilder.Builder, objs []interface{}, names []string) error {
	if len(objs) == 0 || len(names) == 0 {
		return nil
	}

	tree, _ := newPreloadTree(names, "")
	for _, name := range tree.relations {
		relation := findRelation(builder, name)

		var err error
		if relation.Kind == structinfo.BelongsTo {
			err = c.preloadBelongsTo(ctx, relation, objs, tree.paths[name])
		} else {
			err = c.preloadHasMany(ctx, relation, objs, tree.paths[name])
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// preloadNested loads the relations of the related objects named in paths, before the related objects are set
// in their fields, so that the copies set in fields of type []T have them too.
func (c *CRUD) preloadNested(ctx context.Context, relation *structinfo.RelationInfo, related []interface{}, paths []string) error {
	if len(paths) == 0 || len(related) == 0 {
		return nil
	}

	relatedBuilder, err := c.builder(reflect.New(relation.Type).Interface())
	if err != nil {
		return getBuilderObjectCRUDError(err)
	}

	return c.preloadPaths(ctx, relatedBuilder, related, paths)
}

func findRelation(builder *sqlbuilder.Builder, name string) *structinfo.RelationInfo {
	for _, relation := range builder.Relations() {
		if relation.Field == name {
			return relation
		}
	}

	return nil
}

// preloadBelongsTo sets the pointer field of each object to the related object whose ID is in the key field,
// or to nil when the key field is empty or there is no such object. Objects with the same key share the
// related object.
func (c *CRUD) preloadBelongsTo(ctx context.Context, relation *structinfo.RelationInfo, objs []interface{}, paths []string) error {
	keys := map[interface{}]bool{}
	for _, obj := range objs {
		if key, ok := keyValue(reflect.ValueOf(obj).Elem().FieldByName(relation.KeyField)); ok {
			keys[key] = true
		}
	}

	related, err := c.getRelated(ctx, relation.Type, "ID", keys)
	if err != nil {
		return err
	}

	if err := c.preloadNested(ctx, relation, related, paths); err != nil {
		return err
	}

	byID := make(map[interface{}]reflect.Value, len(related))
	for _, relatedObj := range related {
		byID[ObjIDValue(relatedObj)] = reflect.ValueOf(relatedObj)
	}

	for _, obj := range objs {
		objValue := reflect.ValueOf(obj).Elem()
		field := objValue.FieldByName(relation.Field)

		key, ok := keyValue(objValue.FieldByName(relation.KeyField))
		if relatedObj, found := byID[key]; ok && found {
			field.Set(relatedObj)
		} else {
			field.Set(reflect.Zero(field.Type()))
		}
	}

	return nil
}

// preloadHasMany sets the slice field of each object to the related objects that have its ID in their key field,
// ordered by ID. Objects without related objects get an empty slice.
func (c *CRUD) preloadHasMany(ctx context.Context, relation *structinfo.RelationInfo, objs []interface{}, paths []string) error {
	keys := map[interface{}]bool{}
	for _, obj := range objs {
		if !ObjIDIsZero(obj) {
			keys[ObjIDValue(obj)] = true
		}
	}

	related, err := c.getRelated(ctx, relation.Type, relation.KeyField, keys)
	if err != nil {
		return err
	}

	if err := c.preloadNested(ctx, relation, related, paths); err != nil {
		return err
	}

	byKey := map[interface{}][]reflect.Value{}
	for _, relatedObj := range related {
		relatedValue := reflect.ValueOf(relatedObj)
		key, _ := keyValue(relatedValue.Elem().FieldByName(relation.KeyField))
		if !relation.PointerElements {
			relatedValue = relatedValue.Elem()
		}
		byKey[key] = append(byKey[key], relatedValue)
	}

	for _, obj := range objs {
		field := reflect.ValueOf(obj).Elem().FieldByName(relation.Field)
		children := byKey[ObjIDValue(obj)]

		slice := reflect.MakeSlice(field.Type(), 0, len(children))
		slice = reflect.Append(slice, children...)
		field.Set(slice)
	}

	return nil
}

// getRelated returns the objects of type t whose field has one of the keys, ordered by ID.
// The keys are all uint64 or all strings.
func (c *CRUD) getRelated(ctx context.Context, t reflect.Type, field string, keys map[interface{}]bool) ([]interface{}, error) {
	sortedKeys := make([]interface{}, 0, len(keys))
	for key := range keys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		if key, ok := sortedKeys[i].(uint64); ok {
			return key < sortedKeys[j].(uint64)
		}
		return sortedKeys[i].(string) < sortedKeys[j].(string)
	})

	var related []interface{}
	for start := 0; start < len(sortedKeys); start += preloadChunkSize {
		end := min(start+preloadChunkSize, len(sortedKeys))

		objs, _, err := c.Get(ctx, func() interface{} { return reflect.New(t).Interface() }, GetOptions{
			Order:   []string{"ID", "asc"},
			Filters: &sqlfilters.Filters{field: {Op: sqlfilters.OpIn, Val: sortedKeys[start:end]}},
		})
		if err != nil {
			return nil, getCRUDFuncCRUDError("preload "+t.Name(), err)
		}
		related = append(related, objs...)
	}

	return related, nil
}

// keyValue returns the ID in a key field of type uint64 or string, or a pointer to it, and false when it is empty.
func keyValue(field reflect.Value) (interface{}, bool) {
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return nil, false
		}
		field = field.Elem()
	}

	if field.Kind() == reflect.String {
		// a key without a foreign key is not normalized when it is saved
		if id, err := idValue(true, field.String()); err == nil {
			return id, true
		}
	}

	return field.Interface(), !field.IsZero()
}
