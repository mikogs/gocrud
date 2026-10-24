package registry

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq"
	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/gocrudtest"
)

var testUser = "testuser"
var testPassword = "testpass"
var testName = "testdb"
var testDB *sql.DB

var dockerContainer *gocrudtest.Container

var testCRUD *gocrud.CRUD
var testService *CRUD

func TestMain(m *testing.M) {
	dockerContainer, testDB = gocrudtest.CreateDocker(gocrudtest.PostgresDriver(), testUser, testPassword, testName)

	createCRUD()
	createService()
	code := m.Run()

	_ = testDB.Close()
	gocrudtest.RemoveDocker(dockerContainer)
	os.Exit(gocrudtest.CheckLeaks(code))
}

func createCRUD() {
	testCRUD = gocrud.New(testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
}

func createService() {
	testService = New(map[string]func() interface{}{
		"teststruct": func() interface{} {
			return &gocrudtest.TestStruct{}
		},
		"passwordstruct": func() interface{} {
			return &PasswordStruct{}
		},
		"passwordstructtouch": func() interface{} {
			return &PasswordStruct_Touch{}
		},
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
}

func recreateTestStructTable() {
	_ = testCRUD.DropTable(context.Background(), &gocrudtest.TestStruct{})
	_ = testCRUD.CreateTable(context.Background(), &gocrudtest.TestStruct{})
}

func recreatePasswordStructTable() {
	_ = testCRUD.DropTable(context.Background(), &PasswordStruct{})
	_ = testCRUD.CreateTable(context.Background(), &PasswordStruct{})
}

func fatalIfNotFilterViolations(t *testing.T, err error, want map[string]string) {
	t.Helper()

	var filterErr *FilterValidationError
	if !errors.As(err, &filterErr) {
		t.Fatalf("want *FilterValidationError, got %v", err)
	}

	if !reflect.DeepEqual(filterErr.Violations, want) {
		t.Fatalf("got violations %v, want %v", filterErr.Violations, want)
	}
}
