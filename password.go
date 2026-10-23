package gocrud

import (
	"errors"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// compareHashAndPassword compares a password with its hash. It is a variable for the tests.
var compareHashAndPassword = bcrypt.CompareHashAndPassword

// dummyHash returns the hash that a password is compared with when there is no hash to compare it with, so that a
// verification takes as long as with a hash, and its time does not show whether the object exists, or has a password.
var dummyHash = sync.OnceValue(func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("gocrud dummy password"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return hash
})

// verifyPassword returns PassOK when password matches hash, and PassInvalid otherwise. An empty hash, of an object
// without a password, is not a password that can match, but password is compared with the dummy hash anyway.
func verifyPassword(hash string, password string) int {
	if hash == "" {
		_ = compareHashAndPassword(dummyHash(), []byte(password))
		return PassInvalid
	}

	if compareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return PassInvalid
	}
	return PassOK
}

// verifyWithoutObject compares the passwords to verify with the dummy hash when there is no object to verify them with,
// so that the verification takes as long as with an object.
func verifyWithoutObject(passwordFields []string, passwords map[string]string) {
	for _, name := range passwordFields {
		if password, ok := passwords[name]; ok {
			_ = compareHashAndPassword(dummyHash(), []byte(password))
		}
	}
}

// passwordHashError returns the error of hashing the password of a field: a validation error with the violation "max"
// for a password that is longer than bcrypt can hash, 72 bytes, as for a max rule of the validate tag.
func passwordHashError(field string, err error) error {
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return getObjInvalidCRUDError(map[string]string{field: "max"})
	}
	return getCRUDFuncCRUDError("hash password", err)
}
