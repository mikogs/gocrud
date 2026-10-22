package postgres

import "errors"

type QueryBuilderError struct {
	Op  string
	Tag string
	Err error
}

func (e *QueryBuilderError) Error() string {
	return e.Op + ": " + e.Err.Error()
}

func (e *QueryBuilderError) Unwrap() error {
	return e.Err
}

var errIDFieldNotFoundInQueryContainer = errors.New("id field not found in query container")
var errFieldNameNotFound = errors.New("field name not found")
var errFieldIsIgnored = errors.New("field is ignored")
var errFieldInfosIsNil = errors.New("field infos is nil")

var getClauseBuilderError = func(clause, source string, err error) *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get " + clause + " clause from " + source,
		Err: err,
	}
}

var getColumnNameBuilderError = func(source string) *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get column name from " + source + " field",
		Err: errFieldNameNotFound,
	}
}

var getFieldIsIgnoredError = func() *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get field infos",
		Err: errFieldIsIgnored,
	}
}

var getFieldInfosIsNilError = func() *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get field infos",
		Err: errFieldInfosIsNil,
	}
}

var errOrder = errors.New("order slice must have an even number of elements (field, direction pairs)")

var getOrderBuilderError = func() *QueryBuilderError {
	return &QueryBuilderError{
		Op:  "get order",
		Err: errOrder,
	}
}
