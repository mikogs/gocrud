package gocrud

import (
	"reflect"

	sqlbuilder "github.com/mikogs/gocrud/pkg/sql/builder"
)

func (c *CRUD) builder(obj interface{}) (*sqlbuilder.Builder, error) {
	objType := reflect.TypeOf(obj)

	c.buildersMu.RLock()
	builder, ok := c.builders[objType]
	c.buildersMu.RUnlock()
	if ok {
		return builder, nil
	}

	builder, err := sqlbuilder.New(obj, sqlbuilder.Options{
		TableNamePrefix: c.tableNamePrefix,
		TagName:         c.tagName,
		Dialect:         c.dialect,
	})
	if err != nil {
		return nil, err
	}

	c.buildersMu.Lock()
	c.builders[objType] = builder
	c.buildersMu.Unlock()

	return builder, nil
}
