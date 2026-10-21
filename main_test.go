package gocrud

import (
	"context"
	"database/sql"
	"log"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/mikogs/gocrud/pkg/test"
	"github.com/ory/dockertest/v3"
	_ "modernc.org/sqlite"
)

var testUser = "testuser"
var testPassword = "testpass"
var testName = "testdb"
var testDB *sql.DB

var dockerPool *dockertest.Pool
var dockerResource *dockertest.Resource

var testCRUD *CRUD

var testDBSQLite *sql.DB
var testCRUDSQLite *CRUD

func TestMain(m *testing.M) {
	dockerPool, dockerResource, testDB = test.CreateDocker(testUser, testPassword, testName)

	var err error
	testDBSQLite, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatalf("Could not open SQLite in-memory database: %s", err)
	}

	var code int

	defer func() {
		test.RemoveDocker(dockerPool, dockerResource)
		testDBSQLite.Close()
		os.Exit(code)
	}()

	createCRUD()
	createCRUDSQLite()
	code = m.Run()
}

func createCRUD() {
	testCRUD = New(testDB, Options{Dialect: DialectPostgres})
}

func createCRUDSQLite() {
	testCRUDSQLite = New(testDBSQLite, Options{Dialect: DialectSQLite})
}

func recreateTestStructTable() {
	_ = testCRUD.DropTable(context.Background(), &test.TestStruct{})
	_ = testCRUD.CreateTable(context.Background(), &test.TestStruct{})
}

func recreateTestStructTableSQLite() {
	_ = testCRUDSQLite.DropTable(context.Background(), &test.TestStruct{})
	_ = testCRUDSQLite.CreateTable(context.Background(), &test.TestStruct{})
}
