package bench

import (
	"testing"

	"github.com/mikogs/gocrud"
)

// BenchmarkOverhead runs the operations on a fake database that answers at once, so that it shows the work of gocrud
// itself, next to the work of database/sql.
func BenchmarkOverhead(b *testing.B) {
	runOperations(b, func(fakeRows int) *env {
		return newEnv(openFake(fakeRows), gocrud.DialectPostgres)
	})
}
