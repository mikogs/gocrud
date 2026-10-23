package bench

import (
	"os"
	"testing"

	"github.com/mikogs/gocrud/pkg/test"
)

func TestMain(m *testing.M) {
	code := m.Run()

	for _, e := range []*env{sqliteEnv, postgresEnv} {
		if e != nil {
			_ = e.db.Close()
		}
	}
	for _, db := range fakeDBs {
		_ = db.Close()
	}
	os.Exit(test.CheckLeaks(code))
}
