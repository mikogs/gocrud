package bench

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	_ "github.com/lib/pq"
	"github.com/mikogs/gocrud"
	_ "modernc.org/sqlite"
)

var (
	sqliteOnce sync.Once
	sqliteEnv  *env
	sqliteErr  error

	postgresOnce sync.Once
	postgresEnv  *env
	postgresErr  error
)

// BenchmarkSQLite runs the operations on a SQLite database in a file, as a prototype would use it.
func BenchmarkSQLite(b *testing.B) {
	sqliteOnce.Do(func() {
		var dir string
		if dir, sqliteErr = os.MkdirTemp("", "gocrud-bench"); sqliteErr != nil {
			return
		}
		var db *sql.DB
		if db, sqliteErr = sql.Open("sqlite", filepath.Join(dir, "bench.db")); sqliteErr != nil {
			return
		}
		sqliteEnv = newEnv(db, gocrud.DialectSQLite)
		sqliteErr = seed(sqliteEnv)
	})
	if sqliteErr != nil {
		b.Fatal(sqliteErr)
	}

	runOperations(b, func(int) *env { return sqliteEnv })
}

// BenchmarkPostgres runs the operations on the PostgreSQL database of BENCH_POSTGRES_URL, in which it replaces the
// tables with the prefix bench_. It is skipped when the variable is not set.
func BenchmarkPostgres(b *testing.B) {
	url := os.Getenv("BENCH_POSTGRES_URL")
	if url == "" {
		b.Skip("BENCH_POSTGRES_URL is not set")
	}

	postgresOnce.Do(func() {
		var db *sql.DB
		if db, postgresErr = sql.Open("postgres", url); postgresErr != nil {
			return
		}
		postgresEnv = newEnv(db, gocrud.DialectPostgres)
		postgresErr = seed(postgresEnv)
	})
	if postgresErr != nil {
		b.Fatal(postgresErr)
	}

	runOperations(b, func(int) *env { return postgresEnv })
}
