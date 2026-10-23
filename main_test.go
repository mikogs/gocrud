package gocrud

import (
	"context"
	"database/sql"
	"log"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/mikogs/gocrud/pkg/test"
	_ "modernc.org/sqlite"
)

var testUser = "testuser"
var testPassword = "testpass"
var testName = "testdb"
var testDB *sql.DB

var dockerContainer *test.Container

var testCRUD *CRUD

var testDBSQLite *sql.DB
var testCRUDSQLite *CRUD

func TestMain(m *testing.M) {
	dockerContainer, testDB = test.CreateDocker(testUser, testPassword, testName)

	var err error
	testDBSQLite, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatalf("Could not open SQLite in-memory database: %s", err)
	}

	createCRUD()
	createCRUDSQLite()
	code := m.Run()

	_ = testDB.Close()
	_ = testDBSQLite.Close()
	test.RemoveDocker(dockerContainer)
	os.Exit(test.CheckLeaks(code))
}

func createCRUD() {
	testCRUD = New(testDB, Options{Dialect: DialectPostgres})
}

func createCRUDSQLite() {
	testCRUDSQLite = New(testDBSQLite, Options{Dialect: DialectSQLite})
}

// recreateTestStructs creates the table of test.TestStruct again, with the objects returned by newObj for 1 to n.
func recreateTestStructs(t *testing.T, crud *CRUD, n int, newObj func(i int) *test.TestStruct) {
	t.Helper()
	ctx := context.Background()

	_ = crud.DropTable(ctx, &test.TestStruct{})
	if err := crud.CreateTable(ctx, &test.TestStruct{}); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	for i := 1; i <= n; i++ {
		if err := crud.Save(ctx, newObj(i), SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}
}

// recreateTwentyTestStructs creates 20 objects with ages 21 to 40, of which the first 10 have the e-mail
// another@example.com, and the others primary@example.com.
func recreateTwentyTestStructs(t *testing.T, crud *CRUD) {
	t.Helper()

	recreateTestStructs(t, crud, 20, func(i int) *test.TestStruct {
		ts := test.TestStructWithData()
		ts.Age = 20 + i
		if i <= 10 {
			ts.PrimaryEmail = "another@example.com"
		}
		return ts
	})
}

func testStructAges(objs []*test.TestStruct) []int {
	ages := []int{}
	for _, obj := range objs {
		ages = append(ages, obj.Age)
	}
	return ages
}
