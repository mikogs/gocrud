package gocrud

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

// countComparisons replaces compareHashAndPassword with a function that counts the comparisons with the dummy hash and
// with other hashes, until the end of the test.
func countComparisons(t *testing.T) *[2]int {
	t.Helper()

	counts := &[2]int{}
	compare := compareHashAndPassword
	compareHashAndPassword = func(hash []byte, password []byte) error {
		if bytes.Equal(hash, dummyHash()) {
			counts[0]++
		} else {
			counts[1]++
		}
		return compare(hash, password)
	}
	t.Cleanup(func() { compareHashAndPassword = compare })
	return counts
}

func testPasswordVerificationTime(t *testing.T, crud *CRUD) {
	recreatePasswordTable(t, crud)
	ctx := context.Background()

	withPassword := &PasswordStruct{Name: "ida", Password: "secret"}
	withoutPassword := &PasswordStruct{Name: "jack"}
	for _, obj := range []*PasswordStruct{withPassword, withoutPassword, {Name: "kate", Password: "secret"}} {
		savePasswordStruct(t, crud, obj, SaveOptions{})
	}

	load := func(id uint64, password string) map[string]int {
		verified, err := crud.Load(ctx, &PasswordStruct{}, id, LoadOptions{VerifyPasswordFields: map[string]string{"Password": password}})
		if err != nil {
			t.Fatalf("Load failed: %s", err.Error())
		}
		return verified
	}
	get := func(filter sqlfilters.Filter) map[string]int {
		_, verified, err := crud.Get(ctx, func() interface{} { return &PasswordStruct{} }, GetOptions{
			Filters: filter, VerifyPasswordFields: map[string]string{"Password": "secret"},
		})
		if err != nil {
			t.Fatalf("Get failed: %s", err.Error())
		}
		return verified
	}
	byName := func(name string) sqlfilters.Filters {
		return sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: name}}
	}

	tests := []struct {
		name   string
		verify func() interface{}
		want   interface{}
		counts [2]int
	}{
		{"Load of an object with the password", func() interface{} { return load(withPassword.ID, "secret") }, map[string]int{"Password": PassOK}, [2]int{0, 1}},
		{"Load of an object with another password", func() interface{} { return load(withPassword.ID, "wrong") }, map[string]int{"Password": PassInvalid}, [2]int{0, 1}},
		{"Load of an object without a password", func() interface{} { return load(withoutPassword.ID, "") }, map[string]int{"Password": PassInvalid}, [2]int{1, 0}},
		{"Load of an object that does not exist", func() interface{} { return load(1000, "secret") }, map[string]int{}, [2]int{1, 0}},
		{"Get of one object", func() interface{} { return get(byName("ida")) }, map[string]int{"Password": PassOK}, [2]int{0, 1}},
		{"Get of one object without a password", func() interface{} { return get(byName("jack")) }, map[string]int{"Password": PassInvalid}, [2]int{1, 0}},
		{"Get of no objects", func() interface{} { return get(byName("nobody")) }, map[string]int{}, [2]int{1, 0}},
		{"Get of several objects", func() interface{} { return get(nil) }, map[string]int{}, [2]int{1, 0}},
	}
	for _, test := range tests {
		counts := countComparisons(t)
		if got := test.verify(); !reflect.DeepEqual(got, test.want) || *counts != test.counts {
			t.Fatalf("%s returned %v with %d comparisons with the dummy hash and %d with other hashes, want %v with %v",
				test.name, got, counts[0], counts[1], test.want, test.counts)
		}
	}

	counts := countComparisons(t)
	if _, err := crud.Load(ctx, &PasswordStruct{}, 1000, LoadOptions{}); err != nil || *counts != [2]int{} {
		t.Fatalf("Load without passwords to verify compared %v, %v, want nothing", *counts, err)
	}
}

func TestPassword_VerificationTime(t *testing.T) { testPasswordVerificationTime(t, testCRUD) }
func TestSQLite_Password_VerificationTime(t *testing.T) {
	testPasswordVerificationTime(t, testCRUDSQLite)
}
