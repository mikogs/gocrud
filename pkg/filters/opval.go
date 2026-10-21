package filters

type OpVal struct {
	Op  int
	Val interface{}
}

// IsNull checks if the filter is an IS NULL condition (OpEqual with a nil value) or
// an IS NOT NULL condition (OpNotEqual with a nil value).
func (o OpVal) IsNull() bool {
	return (o.Op == OpEqual || o.Op == OpNotEqual) && isNil(o.Val)
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
)
