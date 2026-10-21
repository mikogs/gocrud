package gocrud

import (
	"reflect"
	"strings"

	sqlbuilder "github.com/mikogs/gocrud/pkg/sql/builder"
)

func (c *CRUD) builder(obj interface{}) (*sqlbuilder.Builder, error) {
	name := c.builderName(obj)

	c.buildersMu.RLock()
	builder, ok := c.builders[name]
	c.buildersMu.RUnlock()
	if ok {
		return builder, nil
	}

	builder = sqlbuilder.New(obj, sqlbuilder.Options{
		TableNamePrefix: c.tableNamePrefix,
		TagName:         c.tagName,
		Dialect:         c.dialect,
	})

	c.buildersMu.Lock()
	c.builders[name] = builder
	c.buildersMu.Unlock()

	return builder, nil
}

func (c *CRUD) builderName(obj interface{}) string {
	objValue := reflect.ValueOf(obj)
	objIndirect := reflect.Indirect(objValue)
	objType := objIndirect.Type()

	if objType.String() != "reflect.Value" {
		return objType.Name()
	}

	objType = reflect.ValueOf(obj.(reflect.Value).Interface()).Type().Elem()
	name := objType.Name()

	if !strings.Contains(name, ".") {
		return name
	}

	nameArr := strings.Split(name, ".")
	return nameArr[1]
}
