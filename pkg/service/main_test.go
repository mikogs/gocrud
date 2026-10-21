package service

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/test"
	"github.com/ory/dockertest/v3"
)

var testUser = "testuser"
var testPassword = "testpass"
var testName = "testdb"
var testDB *sql.DB

var dockerPool *dockertest.Pool
var dockerResource *dockertest.Resource

var testCRUD *gocrud.CRUD
var testService *CRUD

func TestMain(m *testing.M) {
	dockerPool, dockerResource, testDB = test.CreateDocker(testUser, testPassword, testName)

	var code int

	defer func() {
		test.RemoveDocker(dockerPool, dockerResource)
		os.Exit(code)
	}()

	createCRUD()
	createService()

	code = m.Run()
}

func createCRUD() {
	testCRUD = gocrud.New(testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
}

func createService() {
	testService = New(map[string]func() interface{}{
		"teststruct": func() interface{} {
			return &test.TestStruct{}
		},
		"passwordstruct": func() interface{} {
			return &PasswordStruct{}
		},
		"passwordstructtouch": func() interface{} {
			return &PasswordStruct_Touch{}
		},
	}, testDB, gocrud.DialectPostgres)
}

func recreateTestStructTable() {
	_ = testCRUD.DropTable(context.Background(), &test.TestStruct{})
	_ = testCRUD.CreateTable(context.Background(), &test.TestStruct{})
}

func recreatePasswordStructTable() {
	_ = testCRUD.DropTable(context.Background(), &PasswordStruct{})
	_ = testCRUD.CreateTable(context.Background(), &PasswordStruct{})
}
