package sqlite

import "errors"

type QueryBuilderError struct {
	Op  string
	Err error
}

func (e *QueryBuilderError) Error() string {
	return e.Op + ": " + e.Err.Error()
}

func (e *QueryBuilderError) Unwrap() error {
	return e.Err
}

var idFieldNotFoundInQueryContainerError = errors.New("id field not found in query container")
var fieldNameNotFoundError = errors.New("field name not found")
var fieldIsIgnoredError = errors.New("field is ignored")
var fieldInfosIsNilError = errors.New("field infos is nil")

var getClauseBuilderError = func(clause, source string, err error) *QueryBuilderError {
	return &QueryBuilderError{Op: "get " + clause + " clause from " + source, Err: err}
}

var getColumnNameBuilderError = func(source string) *QueryBuilderError {
	return &QueryBuilderError{Op: "get column name from " + source + " field", Err: fieldNameNotFoundError}
}

var getFieldIsIgnoredError = func() *QueryBuilderError {
	return &QueryBuilderError{Op: "get field infos", Err: fieldIsIgnoredError}
}

var getFieldInfosIsNilError = func() *QueryBuilderError {
	return &QueryBuilderError{Op: "get field infos", Err: fieldInfosIsNilError}
}

var rawFilterError = errors.New("raw filter value must be an []interface{} starting with a query string")
var orderError = errors.New("order slice must have an even number of elements (field, direction pairs)")

var getRawFilterBuilderError = func() *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get raw filter",
		Err: rawFilterError,
	}
}

var getOrderBuilderError = func() *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get order",
		Err: orderError,
	}
}
