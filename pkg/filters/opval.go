package filters

import "reflect"

type OpVal struct {
	Op  int
	Val interface{}
}

// IsNull checks if the filter is an IS NULL condition (OpEqual with a nil value) or
// an IS NOT NULL condition (OpNotEqual with a nil value).
func (o OpVal) IsNull() bool {
	return (o.Op == OpEqual || o.Op == OpNotEqual) && isNil(o.Val)
}

// InValues returns the values of an OpIn filter, which is a slice or an array, e.g. []uint64{1, 2}.
// It returns false when the value is not a slice or an array.
func (o OpVal) InValues() ([]interface{}, bool) {
	val := reflect.ValueOf(o.Val)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return nil, false
	}

	values := make([]interface{}, val.Len())
	for i := range values {
		values[i] = val.Index(i).Interface()
	}

	return values, true
}

const (
	OpOR = iota * 1
	OpAND
)

const (
	OpEqual = iota * 1
	OpNotEqual
	OpLike
	OpMatch
	OpGreater
	OpLower
	OpGreaterOrEqual
	OpLowerOrEqual
	OpBit
	// OpIn matches the values in a slice or an array, e.g. []string{"paid", "shipped"}. An empty slice matches nothing.
	OpIn
)
