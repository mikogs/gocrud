package gocrud

import (
	"bytes"
	"context"
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

	verify := func(filter sqlfilters.Filter, password string) bool {
		verified, err := crud.VerifyPasswords(ctx, &PasswordStruct{}, VerifyPasswordsOptions{
			Filters: filter, Passwords: map[string]string{"Password": password},
		})
		if err != nil {
			t.Fatalf("VerifyPasswords failed: %s", err.Error())
		}
		return verified
	}
	byName := func(name string) sqlfilters.Filters {
		return sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: name}}
	}

	tests := []struct {
		name   string
		filter sqlfilters.Filter
		pass   string
		want   bool
		counts [2]int
	}{
		{"an object with the password", byName("ida"), "secret", true, [2]int{0, 1}},
		{"an object with another password", byName("ida"), "wrong", false, [2]int{0, 1}},
		{"an object without a password", byName("jack"), "", false, [2]int{1, 0}},
		{"no objects", byName("nobody"), "secret", false, [2]int{1, 0}},
		{"several objects", nil, "secret", false, [2]int{1, 0}},
	}
	for _, test := range tests {
		counts := countComparisons(t)
		if got := verify(test.filter, test.pass); got != test.want || *counts != test.counts {
			t.Fatalf("VerifyPasswords of %s returned %t with %d comparisons with the dummy hash and %d with other hashes, want %t with %v",
				test.name, got, counts[0], counts[1], test.want, test.counts)
		}
	}

	counts := countComparisons(t)
	if err := crud.Load(ctx, &PasswordStruct{}, 1000, LoadOptions{}); err != nil || *counts != [2]int{} {
		t.Fatalf("Load compared %v, %v, want nothing", *counts, err)
	}
}

func TestPassword_VerificationTime(t *testing.T) { testPasswordVerificationTime(t, testCRUD) }
func TestSQLite_Password_VerificationTime(t *testing.T) {
	testPasswordVerificationTime(t, testCRUDSQLite)
}
