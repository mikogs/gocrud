package gocrud

import (
	"reflect"

	sqlbuilder "github.com/mikogs/gocrud/pkg/sql/builder"
)

func (c *CRUD) builder(obj interface{}) (*sqlbuilder.Builder, error) {
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
	})
	if err != nil {
		return nil, err
	}

	c.builders.mu.Lock()
	c.builders.builders[objType] = builder
	c.builders.mu.Unlock()

	return builder, nil
}
