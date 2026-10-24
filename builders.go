package gocrud

import (
	"reflect"

	"github.com/mikogs/gocrud/internal/sqlbuilder"
)

func (c *CRUD) builder(obj any) (*sqlbuilder.Builder, error) {
	objType := reflect.TypeOf(obj)

	c.builders.mu.RLock()
	builder, ok := c.builders.builders[objType]
	c.builders.mu.RUnlock()
	if ok {
		return builder, nil
	}

	builder, err := sqlbuilder.New(obj, sqlbuilder.Options{
		TableNamePrefix: c.tableNamePrefix,
		TagName:         c.tagName,
		Dialect:         c.dialect,
		RelatedBuilder: func(t reflect.Type) (*sqlbuilder.Builder, error) {
			return c.builder(reflect.New(t).Interface())
		},
	})
	if err != nil {
		return nil, err
	}

	if err := checkQueryMethods(obj); err != nil {
		return nil, err
	}

	c.builders.mu.Lock()
	c.builders.builders[objType] = builder
	c.builders.mu.Unlock()

	return builder, nil
}
