package gocrud

const (
	IDField = "ID"
)

// The results of the verification of a password. PassInvalid is 0, the zero value, so that a password that was not
// verified, e.g. the missing key of a field when no object or more than one is found, is never PassOK.
const (
	PassInvalid = iota
	PassOK
)
