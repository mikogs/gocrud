package service

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// ErrForeignKeyCycle is returned when the foreign keys of the registered structs reference each other in a cycle,
// so their tables cannot be created one after another.
var ErrForeignKeyCycle = errors.New("foreign keys reference each other in a cycle")

// tableKeys returns the registry keys of the structs that own a table, i.e. are not projections of other structs,
// ordered so that the tables referenced by foreign keys come before the tables that reference them.
func (c *CRUD) tableKeys() ([]string, error) {
	keys := make([]string, 0, len(c.registry))
	for key, constructor := range c.registry {
		typ := reflect.TypeOf(constructor())
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if !strings.Contains(typ.Name(), "_") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	keyOfTable := map[string]string{}
	references := map[string][]string{}
	for _, key := range keys {
		obj := c.registry[key]()

		table, err := c.crud.TableName(obj)
		if err != nil {
			return nil, err
		}
		keyOfTable[table] = key

		references[key], err = c.crud.ReferencedTables(obj)
		if err != nil {
			return nil, err
		}
	}

	const (
		visiting = 1
		visited  = 2
	)
	state := map[string]int{}
	ordered := make([]string, 0, len(keys))

	var visit func(key string, path []string) error
	visit = func(key string, path []string) error {
		switch state[key] {
		case visited:
			return nil
		case visiting:
			return fmt.Errorf("%w: %s", ErrForeignKeyCycle, strings.Join(append(path, key), " -> "))
		}

		state[key] = visiting
		for _, table := range references[key] {
			// a table that references itself, or a table that is not in the registry, does not change the order
			referencedKey, ok := keyOfTable[table]
			if !ok || referencedKey == key {
				continue
			}
			if err := visit(referencedKey, append(path, key)); err != nil {
				return err
			}
		}
		state[key] = visited
		ordered = append(ordered, key)

		return nil
	}

	for _, key := range keys {
		if err := visit(key, nil); err != nil {
			return nil, err
		}
	}

	return ordered, nil
}
