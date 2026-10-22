package builder

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mikogs/gocrud/pkg/filters"
	"github.com/mikogs/gocrud/pkg/structinfo"
)

type TestStruct struct {
	ID             uint64 `json:"id"`
	Flags          uint64 `json:"flags"`
	PrimaryEmail   string `json:"email"`
	EmailSecondary string `json:"email2"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Age            int    `json:"age"`
	Price          int    `json:"price"`
	PostCode       string `json:"post_code"`
	PostCode2      string `json:"post_code2"`
	Password       string `json:"password"`
	CreatedBy      uint64 `json:"created_by"`
	Key            string `json:"key" sql:"uniq type:varchar(2000)"`
}

type TestStructWithCustomSelect struct {
	ID    uint64 `json:"id"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

func (s *TestStructWithCustomSelect) BuildSelectPrefixQuery(tableNamePrefix string) (string, error) {
	return `SELECT id, name, price FROM "` + tableNamePrefix + `test_struct_with_custom_select" t1 INNER JOIN prices t2 ON t1.id=t2.test_struct_with_custom_select_id`, nil
}

type TestStructWithIgnoredFields struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	IgnoreMe  string `json:"-" sql:"-"`
	Price     int64  `json:"price"`
	IgnoreMe2 string `json:"-" sql:"-"`
}

type TestStructWithIndexes struct {
	ID        uint64 `json:"id"`
	Email     string `json:"email" sql:"idx"`
	FirstName string `json:"first_name" sql:"idx:name_idx"`
	LastName  string `json:"last_name" sql:"idx:name_idx"`
	Code      string `json:"code" sql:"uidx:code_idx"`
}

var testStructObj = &TestStruct{}
var testStructWithCustomSelectObj = &TestStructWithCustomSelect{}
var testStructWithIgnoredFieldsObj = &TestStructWithIgnoredFields{}
var testStructWithIndexesObj = &TestStructWithIndexes{}

func TestSQLQueries(t *testing.T) {
	h := mustNew(t, testStructObj, Options{})

	got := h.DropTable()
	want := `DROP TABLE IF EXISTS "test_struct";`
	if got != want {
		t.Fatalf("Want %v\ngot %v", want, got)
	}

	got = h.CreateTable()
	want = `CREATE TABLE IF NOT EXISTS "test_struct" ("id" SERIAL PRIMARY KEY,"flags" BIGINT NOT NULL DEFAULT 0,` +
		`"primary_email" VARCHAR(255) NOT NULL DEFAULT '',"email_secondary" VARCHAR(255) NOT NULL DEFAULT '',` +
		`"first_name" VARCHAR(255) NOT NULL DEFAULT '',"last_name" VARCHAR(255) NOT NULL DEFAULT '',` +
		`"age" BIGINT NOT NULL DEFAULT 0,"price" BIGINT NOT NULL DEFAULT 0,"post_code" VARCHAR(255) NOT NULL DEFAULT '',` +
		`"post_code2" VARCHAR(255) NOT NULL DEFAULT '',"password" VARCHAR(255) NOT NULL DEFAULT '',` +
		`"created_by" BIGINT NOT NULL DEFAULT 0,"key" VARCHAR(2000) NOT NULL DEFAULT '' UNIQUE);`
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLInsertQueries(t *testing.T) {
	h := mustNew(t, testStructObj, Options{})

	got := h.Insert()
	want := `INSERT INTO "test_struct"("flags","primary_email","email_secondary","first_name","last_name",` +
		`"age","price","post_code","post_code2","password","created_by","key") ` +
		`VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING "id";`
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLUpdateByIDQueries(t *testing.T) {
	h := mustNew(t, testStructObj, Options{})

	got := h.UpdateByID()
	want := `UPDATE "test_struct" SET "flags"=$1,"primary_email"=$2,"email_secondary"=$3,"first_name"=$4,"last_name"=$5,` +
		`"age"=$6,"price"=$7,"post_code"=$8,"post_code2"=$9,"password"=$10,"created_by"=$11,"key"=$12 WHERE "id" = $13;`
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLInsertOnConflictUpdateQueries(t *testing.T) {
	h := mustNew(t, testStructObj, Options{})

	got := h.InsertOnConflictUpdate()
	want := `INSERT INTO "test_struct"("id","flags","primary_email","email_secondary","first_name","last_name",` +
		`"age","price","post_code","post_code2","password","created_by","key") ` +
		`VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ` +
		`ON CONFLICT ("id") DO UPDATE SET ` +
		`"flags"=$14,"primary_email"=$15,"email_secondary"=$16,"first_name"=$17,"last_name"=$18,"age"=$19,` +
		`"price"=$20,"post_code"=$21,"post_code2"=$22,"password"=$23,"created_by"=$24,"key"=$25 ` +
		`RETURNING "id";`
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLDeleteQueries(t *testing.T) {
	h := mustNew(t, testStructObj, Options{})

	got := h.DeleteByID()
	want := `DELETE FROM "test_struct" WHERE "id" = $1;`
	if got != want {
		t.Fatalf("\nwant %v\ngot %v", want, got)
	}
}

func TestSQLSelectQueries(t *testing.T) {
	h := mustNew(t, testStructObj, Options{})

	selectPrefix := `SELECT "id","flags","primary_email","email_secondary","first_name","last_name",` +
		`"age","price","post_code","post_code2","password","created_by","key" FROM "test_struct"`

	got := h.SelectByID()
	want := selectPrefix + ` WHERE "id" = $1;`
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}

	got, err := h.Select(nil, 67, 13, nil)
	want = selectPrefix + " LIMIT 67 OFFSET 13;"
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}

	got, err = h.Select([]string{"EmailSecondary", "desc", "Age", "asc"}, 67, 13, &filters.Filters{
		"Price":     {Op: filters.OpEqual, Val: 4444},
		"PostCode2": {Op: filters.OpEqual, Val: "11-111"},
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	want = selectPrefix + ` WHERE "post_code2"=$1 AND "price"=$2 ORDER BY "email_secondary" DESC,"age" ASC LIMIT 67 OFFSET 13;`
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}

	got, err = h.Select([]string{"EmailSecondary", "asc", "Age", "desc"}, 1, 3, filters.Or(filters.Filters{
		"Price":     {Op: filters.OpEqual, Val: 33},
		"PostCode2": {Op: filters.OpEqual, Val: "11-111"},
	}, filters.Raw(".Price=? OR (.EmailSecondary=? AND .Age IN (?)) OR (.Age IN (?)) OR (.EmailSecondary IN (?))", 0, 0, []int{0, 0, 0, 0}, []int{0, 0, 0}, []int{0, 0})))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	want = selectPrefix + ` WHERE` +
		` ("post_code2"=$1 AND "price"=$2) OR ("price"=$3 OR ("email_secondary"=$4 AND "age" IN ($5,$6,$7,$8)) OR ("age" IN ($9,$10,$11)) OR ("email_secondary" IN ($12,$13)))` +
		` ORDER BY "email_secondary" ASC,"age" DESC LIMIT 1 OFFSET 3;`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}

	got, err = h.Select([]string{"EmailSecondary", "desc"}, 61, 19, &filters.Filters{
		"Price":     {Op: filters.OpGreater, Val: 4443},
		"PostCode2": {Op: filters.OpLike, Val: "11%"},
		"Age":       {Op: filters.OpLowerOrEqual, Val: 99},
		"FirstName": {Op: filters.OpMatch, Val: "^[A-Z][a-z]+$"},
		"CreatedBy": {Op: filters.OpGreaterOrEqual, Val: 100},
		"Flags":     {Op: filters.OpBit, Val: 8},
	})
	want = selectPrefix + ` WHERE "age"<=$1 AND "created_by">=$2 AND "first_name" ~ $3 AND "flags"&$4>0 AND "post_code2" LIKE $5 AND "price">$6` +
		` ORDER BY "email_secondary" DESC LIMIT 61 OFFSET 19;`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLSelectCountQueries(t *testing.T) {
	h := mustNew(t, testStructObj, Options{})

	got, err := h.SelectCount(&filters.Filters{
		"Price": {Op: filters.OpEqual, Val: 4444},
	})
	want := `SELECT COUNT(*) AS cnt FROM "test_struct" WHERE "price"=$1;`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLDeleteWithFiltersQueries(t *testing.T) {
	h := mustNew(t, testStructObj, Options{})

	got, err := h.Delete(&filters.Filters{
		"Price": {Op: filters.OpEqual, Val: 4444},
	})
	want := `DELETE FROM "test_struct" WHERE "price"=$1;`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}

	got, err = h.Delete(
		filters.And(filters.Filters{
			"Price": {Op: filters.OpEqual, Val: 4444},
		}, filters.Raw(".Price=? OR .EmailSecondary=? OR .Age IN (?)", 0, 0, []int{0, 0, 0})))
	want = `DELETE FROM "test_struct" WHERE "price"=$1 AND ` +
		`("price"=$2 OR "email_secondary"=$3 OR "age" IN ($4,$5,$6));`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}

	got, err = h.Delete(
		filters.Raw(".Price=? OR .EmailSecondary=? OR .Age IN (?)", 0, 0, []int{0, 0, 0}))
	want = `DELETE FROM "test_struct" WHERE "price"=$1 OR "email_secondary"=$2 OR "age" IN ($3,$4,$5);`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}

	got, err = h.DeleteReturningID(
		filters.Raw(".Price=? OR .EmailSecondary=? OR .Age IN (?)", 0, 0, []int{0, 0, 0}))
	want = `DELETE FROM "test_struct" WHERE "price"=$1 OR "email_secondary"=$2 OR "age" IN ($3,$4,$5) RETURNING "id";`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLUpdateQueries(t *testing.T) {
	h := mustNew(t, testStructObj, Options{})

	got, err := h.Update(
		map[string]interface{}{"Price": 1234, "PostCode2": "12-345"},
		&filters.Filters{
			"PrimaryEmail": {Op: filters.OpEqual, Val: "primary@example.com"},
		},
	)
	want := `UPDATE "test_struct" SET "post_code2"=$1,"price"=$2 WHERE "primary_email"=$3;`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}

	got, err = h.Update(
		map[string]interface{}{"FirstName": "Jane", "LastName": "Doe"},
		&filters.Filters{
			"PostCode": {Op: filters.OpEqual, Val: "11-111"},
		},
	)
	want = `UPDATE "test_struct" SET "first_name"=$1,"last_name"=$2 WHERE "post_code"=$3;`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLCustomSelectQueries(t *testing.T) {
	h := mustNew(t, testStructWithCustomSelectObj, Options{})

	selectPrefix := `SELECT id, name, price FROM "test_struct_with_custom_select" t1 ` +
		`INNER JOIN prices t2 ON t1.id=t2.test_struct_with_custom_select_id`

	got, err := h.Select(nil, 67, 13, nil)
	want := selectPrefix + " LIMIT 67 OFFSET 13;"
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLIgnoredFields(t *testing.T) {
	h := mustNew(t, testStructWithIgnoredFieldsObj, Options{})

	got := h.CreateTable()
	want := `CREATE TABLE IF NOT EXISTS "test_struct_with_ignored_fields" ("id" SERIAL PRIMARY KEY,"name" VARCHAR(255) NOT NULL DEFAULT '',` +
		`"price" BIGINT NOT NULL DEFAULT 0);`
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLIndexes(t *testing.T) {
	h := mustNew(t, testStructWithIndexesObj, Options{})

	got := h.CreateTable()
	want := `CREATE TABLE IF NOT EXISTS "test_struct_with_indexes" ("id" SERIAL PRIMARY KEY,"email" VARCHAR(255) NOT NULL DEFAULT '',` +
		`"first_name" VARCHAR(255) NOT NULL DEFAULT '',"last_name" VARCHAR(255) NOT NULL DEFAULT '',"code" VARCHAR(255) NOT NULL DEFAULT ''); ` +
		`CREATE INDEX IF NOT EXISTS "idx_test_struct_with_indexes_email" ON "test_struct_with_indexes" ("email"); ` +
		`CREATE INDEX IF NOT EXISTS "idx_test_struct_with_indexes_name_idx" ON "test_struct_with_indexes" ("first_name","last_name"); ` +
		`CREATE UNIQUE INDEX IF NOT EXISTS "idx_test_struct_with_indexes_code_idx" ON "test_struct_with_indexes" ("code");`
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

type TestStructWithAliases struct {
	ID    uint64 `sql:"name:t1.id"`
	Name  string `sql:"name:t1.name idx"`
	Price int64  `sql:"name:t2.price"`
}

func (s *TestStructWithAliases) BuildSelectPrefixQuery(tableNamePrefix string) (string, error) {
	return `SELECT t1.id, t1.name, t2.price FROM "` + tableNamePrefix + `test_struct_with_aliases" t1 ` +
		`INNER JOIN prices t2 ON t1.id=t2.test_struct_with_aliases_id`, nil
}

func (s *TestStructWithAliases) BuildSelectCountPrefixQuery(tableNamePrefix string) (string, error) {
	return `SELECT COUNT(*) AS cnt FROM "` + tableNamePrefix + `test_struct_with_aliases" t1 ` +
		`INNER JOIN prices t2 ON t1.id=t2.test_struct_with_aliases_id`, nil
}

type TestStructWithAliasesWithoutCustomSelect struct {
	ID    uint64 `sql:"name:t1.id"`
	Name  string `sql:"name:t1.name idx"`
	Price int64  `sql:"name:t2.price"`
}

var aliasesTestFilters = filters.And(filters.Filters{
	"Name":  {Op: filters.OpLike, Val: "a%"},
	"Price": {Op: filters.OpGreater, Val: 1},
}, filters.Raw(".Price < ?", 100))

func TestSQLAliasedColumnQueries(t *testing.T) {
	h := mustNew(t, &TestStructWithAliases{}, Options{})

	if !h.HasAliasedColumnNames() {
		t.Fatal("HasAliasedColumnNames returned false for a struct with aliased column names")
	}

	join := `FROM "test_struct_with_aliases" t1 INNER JOIN prices t2 ON t1.id=t2.test_struct_with_aliases_id`
	where := `WHERE (t1."name" LIKE $1 AND t2."price">$2) AND (t2."price" < $3)`
	whereWithoutAliases := `WHERE ("name" LIKE $2 AND "price">$3) AND ("price" < $4)`

	selectQuery, _ := h.Select([]string{"Price", "desc"}, 10, 0, aliasesTestFilters)
	selectCountQuery, _ := h.SelectCount(aliasesTestFilters)
	updateQuery, _ := h.Update(map[string]interface{}{"Price": 5}, aliasesTestFilters)
	deleteQuery, _ := h.DeleteReturningID(aliasesTestFilters)

	tests := map[string][2]string{
		"CreateTable": {h.CreateTable(), `CREATE TABLE IF NOT EXISTS "test_struct_with_aliases" ("id" SERIAL PRIMARY KEY,` +
			`"name" VARCHAR(255) NOT NULL DEFAULT '',"price" BIGINT NOT NULL DEFAULT 0); ` +
			`CREATE INDEX IF NOT EXISTS "idx_test_struct_with_aliases_name" ON "test_struct_with_aliases" ("name");`},
		"Insert":            {h.Insert(), `INSERT INTO "test_struct_with_aliases"("name","price") VALUES ($1,$2) RETURNING "id";`},
		"UpdateByID":        {h.UpdateByID(), `UPDATE "test_struct_with_aliases" SET "name"=$1,"price"=$2 WHERE "id" = $3;`},
		"SelectByID":        {h.SelectByID(), `SELECT "id","name","price" FROM "test_struct_with_aliases" WHERE "id" = $1;`},
		"Select":            {selectQuery, `SELECT t1.id, t1.name, t2.price ` + join + ` ` + where + ` ORDER BY t2."price" DESC LIMIT 10;`},
		"SelectCount":       {selectCountQuery, `SELECT COUNT(*) AS cnt ` + join + ` ` + where + `;`},
		"Update":            {updateQuery, `UPDATE "test_struct_with_aliases" SET "price"=$1 ` + whereWithoutAliases + `;`},
		"DeleteReturningID": {deleteQuery, `DELETE FROM "test_struct_with_aliases" WHERE ("name" LIKE $1 AND "price">$2) AND ("price" < $3) RETURNING "id";`},
	}

	for name, gotWant := range tests {
		t.Run(name, func(t *testing.T) {
			if gotWant[0] != gotWant[1] {
				t.Fatalf("\nwant %v\ngot  %v", gotWant[1], gotWant[0])
			}
		})
	}
}

func TestSQLAliasedColumnQueries_WithoutCustomSelect(t *testing.T) {
	h := mustNew(t, &TestStructWithAliasesWithoutCustomSelect{}, Options{})

	got, err := h.Select([]string{"Price", "desc"}, 10, 0, aliasesTestFilters)
	want := `SELECT "id","name","price" FROM "test_struct_with_aliases_without_custom_select" ` +
		`WHERE ("name" LIKE $1 AND "price">$2) AND ("price" < $3) ORDER BY "price" DESC LIMIT 10;`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}
}

func TestSQLiteAliasedColumnQueries(t *testing.T) {
	h := mustNew(t, &TestStructWithAliases{}, Options{Dialect: DialectSQLite})

	join := `FROM "test_struct_with_aliases" t1 INNER JOIN prices t2 ON t1.id=t2.test_struct_with_aliases_id`
	where := `WHERE (t1."name" LIKE ? AND t2."price">?) AND (t2."price" < ?)`

	selectQuery, _ := h.Select([]string{"Price", "desc"}, 10, 0, aliasesTestFilters)
	selectCountQuery, _ := h.SelectCount(aliasesTestFilters)
	updateQuery, _ := h.Update(map[string]interface{}{"Price": 5}, aliasesTestFilters)
	deleteQuery, _ := h.DeleteReturningID(aliasesTestFilters)

	tests := map[string][2]string{
		"CreateTable": {h.CreateTable(), `CREATE TABLE IF NOT EXISTS "test_struct_with_aliases" ("id" INTEGER PRIMARY KEY,` +
			`"name" TEXT NOT NULL DEFAULT '',"price" INTEGER NOT NULL DEFAULT 0); ` +
			`CREATE INDEX IF NOT EXISTS "idx_test_struct_with_aliases_name" ON "test_struct_with_aliases" ("name");`},
		"Insert":            {h.Insert(), `INSERT INTO "test_struct_with_aliases"("name","price") VALUES (?,?) RETURNING "id";`},
		"Select":            {selectQuery, `SELECT t1.id, t1.name, t2.price ` + join + ` ` + where + ` ORDER BY t2."price" DESC LIMIT 10;`},
		"SelectCount":       {selectCountQuery, `SELECT COUNT(*) AS cnt ` + join + ` ` + where + `;`},
		"Update":            {updateQuery, `UPDATE "test_struct_with_aliases" SET "price"=? WHERE ("name" LIKE ? AND "price">?) AND ("price" < ?);`},
		"DeleteReturningID": {deleteQuery, `DELETE FROM "test_struct_with_aliases" WHERE ("name" LIKE ? AND "price">?) AND ("price" < ?) RETURNING "id";`},
	}

	for name, gotWant := range tests {
		t.Run(name, func(t *testing.T) {
			if gotWant[0] != gotWant[1] {
				t.Fatalf("\nwant %v\ngot  %v", gotWant[1], gotWant[0])
			}
		})
	}
}

type TestStructWithCustomDeleteByID struct {
	ID   uint64
	Name string
}

func (s *TestStructWithCustomDeleteByID) BuildDeleteByIDQuery(tableNamePrefix string) (string, error) {
	return `UPDATE "` + tableNamePrefix + `test_struct_with_custom_delete_by_id" SET "name"='deleted' WHERE "id" = $1`, nil
}

func TestSQLCustomDeleteByIDQuery(t *testing.T) {
	for _, dialect := range []string{DialectPostgres, DialectSQLite} {
		t.Run(dialect, func(t *testing.T) {
			h := mustNew(t, &TestStructWithCustomDeleteByID{}, Options{Dialect: dialect})

			want := `UPDATE "test_struct_with_custom_delete_by_id" SET "name"='deleted' WHERE "id" = $1;`
			if got := h.DeleteByID(); got != want {
				t.Fatalf("\nwant %v\ngot  %v", want, got)
			}

			wantPrefix := `CREATE TABLE IF NOT EXISTS "test_struct_with_custom_delete_by_id"`
			if got := h.CreateTable(); !strings.HasPrefix(got, wantPrefix) {
				t.Fatalf("CreateTable returned %v, want a query starting with %v", got, wantPrefix)
			}
		})
	}
}

func mustNew(t *testing.T, obj interface{}, options Options) *Builder {
	t.Helper()

	b, err := New(obj, options)
	if err != nil {
		t.Fatalf("New failed: %s", err.Error())
	}

	return b
}

func TestNew_InvalidStruct(t *testing.T) {
	type withoutID struct {
		Name string
	}

	for _, dialect := range []string{DialectPostgres, DialectSQLite} {
		t.Run(dialect, func(t *testing.T) {
			b, err := New(&withoutID{}, Options{Dialect: dialect})
			if !errors.Is(err, structinfo.IDFieldError) || b != nil {
				t.Fatalf("New returned %v, %v, want nil, %v", b, err, structinfo.IDFieldError)
			}

			b, err = New(nil, Options{Dialect: dialect})
			if !errors.Is(err, structinfo.NotStructPointerError) || b != nil {
				t.Fatalf("New returned %v, %v, want nil, %v", b, err, structinfo.NotStructPointerError)
			}
		})
	}
}

func TestSQLInvalidRawFilters(t *testing.T) {
	tests := map[string]filters.Filter{
		"too few values":  filters.Raw(".Price > ? AND .Age < ?", 1),
		"too many values": filters.Raw(".Price > ?", 1, 2),
		"unknown field":   filters.Raw(".Unknown > ?", 1),
	}

	for _, dialect := range []string{DialectPostgres, DialectSQLite} {
		h := mustNew(t, testStructObj, Options{Dialect: dialect})

		for name, f := range tests {
			t.Run(dialect+"/"+name, func(t *testing.T) {

				if _, err := h.Select(nil, 0, 0, f); err == nil {
					t.Fatal("Select did not return an err for an invalid raw filter")
				}
				if _, err := h.SelectCount(f); err == nil {
					t.Fatal("SelectCount did not return an err for an invalid raw filter")
				}
				if _, err := h.DeleteReturningID(f); err == nil {
					t.Fatal("DeleteReturningID did not return an err for an invalid raw filter")
				}
				if _, err := h.Update(map[string]interface{}{"Price": 1}, f); err == nil {
					t.Fatal("Update did not return an err for an invalid raw filter")
				}
			})
		}
	}
}

func TestSQLRawFilterWithNilValue(t *testing.T) {
	f := filters.Raw(".Price = ? OR .Age = ?", nil, 5)

	wants := map[string]string{
		DialectPostgres: `SELECT COUNT(*) AS cnt FROM "test_struct" WHERE "price" = $1 OR "age" = $2;`,
		DialectSQLite:   `SELECT COUNT(*) AS cnt FROM "test_struct" WHERE "price" = ? OR "age" = ?;`,
	}
	for dialect, want := range wants {
		t.Run(dialect, func(t *testing.T) {
			got, err := mustNew(t, testStructObj, Options{Dialect: dialect}).SelectCount(f)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if got != want {
				t.Fatalf("\nwant %v\ngot  %v", want, got)
			}
		})
	}
}

func TestSQLOddOrder(t *testing.T) {
	for _, dialect := range []string{DialectPostgres, DialectSQLite} {
		t.Run(dialect, func(t *testing.T) {
			_, err := mustNew(t, testStructObj, Options{Dialect: dialect}).Select([]string{"Age", "asc", "Price"}, 0, 0, nil)
			if err == nil {
				t.Fatal("Select did not return an err for an order slice with an odd number of elements")
			}
		})
	}
}

type TestStructWithTimes struct {
	ID       uint64
	Name     string
	StartsAt time.Time `sql:"idx"`
}

func TestSQLTimeColumns(t *testing.T) {
	wants := map[string]string{
		DialectPostgres: `CREATE TABLE IF NOT EXISTS "test_struct_with_times" ("id" SERIAL PRIMARY KEY,"name" VARCHAR(255) NOT NULL DEFAULT '',` +
			`"starts_at" TIMESTAMPTZ NOT NULL DEFAULT '0001-01-01 00:00:00+00'); ` +
			`CREATE INDEX IF NOT EXISTS "idx_test_struct_with_times_starts_at" ON "test_struct_with_times" ("starts_at");`,
		DialectSQLite: `CREATE TABLE IF NOT EXISTS "test_struct_with_times" ("id" INTEGER PRIMARY KEY,"name" TEXT NOT NULL DEFAULT '',` +
			`"starts_at" DATETIME NOT NULL DEFAULT '0001-01-01 00:00:00.000000000+00:00'); ` +
			`CREATE INDEX IF NOT EXISTS "idx_test_struct_with_times_starts_at" ON "test_struct_with_times" ("starts_at");`,
	}

	for dialect, want := range wants {
		t.Run(dialect, func(t *testing.T) {
			if got := mustNew(t, &TestStructWithTimes{}, Options{Dialect: dialect}).CreateTable(); got != want {
				t.Fatalf("\nwant %v\ngot  %v", want, got)
			}
		})
	}
}

type TestStructWithModificationFields struct {
	ID         uint64
	Name       string
	CreatedAt  time.Time
	CreatedBy  uint64
	ModifiedAt time.Time
	ModifiedBy uint64
}

func TestSQLModificationFieldsNotUpdated(t *testing.T) {
	tests := map[string][2]string{
		DialectPostgres: {
			`UPDATE "test_struct_with_modification_fields" SET "name"=$1,"modified_at"=$2,"modified_by"=$3 WHERE "id" = $4;`,
			`INSERT INTO "test_struct_with_modification_fields"("id","name","created_at","created_by","modified_at","modified_by") ` +
				`VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT ("id") DO UPDATE SET "name"=$7,"modified_at"=$8,"modified_by"=$9 RETURNING "id";`,
		},
		DialectSQLite: {
			`UPDATE "test_struct_with_modification_fields" SET "name"=?,"modified_at"=?,"modified_by"=? WHERE "id" = ?;`,
			`INSERT INTO "test_struct_with_modification_fields"("id","name","created_at","created_by","modified_at","modified_by") ` +
				`VALUES (?,?,?,?,?,?) ON CONFLICT ("id") DO UPDATE SET "name"=?,"modified_at"=?,"modified_by"=? RETURNING "id";`,
		},
	}

	for dialect, want := range tests {
		t.Run(dialect, func(t *testing.T) {
			h := mustNew(t, &TestStructWithModificationFields{}, Options{Dialect: dialect})
			if got := h.UpdateByID(); got != want[0] {
				t.Fatalf("\nwant %v\ngot  %v", want[0], got)
			}
			if got := h.InsertOnConflictUpdate(); got != want[1] {
				t.Fatalf("\nwant %v\ngot  %v", want[1], got)
			}
		})
	}
}

type TestStructWithNullableFields struct {
	ID        uint64
	Name      string
	Nickname  *string `sql:"uniq"`
	Bio       *string `sql:"type:text"`
	Age       *int64
	Score     *float64
	Rate      float32
	Verified  *bool
	BirthDate *time.Time
}

func TestSQLNullableColumns(t *testing.T) {
	wants := map[string]string{
		DialectPostgres: `CREATE TABLE IF NOT EXISTS "test_struct_with_nullable_fields" ("id" SERIAL PRIMARY KEY,` +
			`"name" VARCHAR(255) NOT NULL DEFAULT '',"nickname" VARCHAR(255) UNIQUE,"bio" TEXT,"age" BIGINT,` +
			`"score" DOUBLE PRECISION,"rate" REAL NOT NULL DEFAULT 0,"verified" BOOLEAN,"birth_date" TIMESTAMPTZ);`,
		DialectSQLite: `CREATE TABLE IF NOT EXISTS "test_struct_with_nullable_fields" ("id" INTEGER PRIMARY KEY,` +
			`"name" TEXT NOT NULL DEFAULT '',"nickname" TEXT UNIQUE,"bio" TEXT,"age" INTEGER,` +
			`"score" REAL,"rate" REAL NOT NULL DEFAULT 0,"verified" INTEGER,"birth_date" DATETIME);`,
	}

	for dialect, want := range wants {
		t.Run(dialect, func(t *testing.T) {
			if got := mustNew(t, &TestStructWithNullableFields{}, Options{Dialect: dialect}).CreateTable(); got != want {
				t.Fatalf("\nwant %v\ngot  %v", want, got)
			}
		})
	}
}

func TestSQLNullConditions(t *testing.T) {
	f := filters.And(filters.Filters{
		"Age":       {Op: filters.OpEqual, Val: nil},
		"BirthDate": {Op: filters.OpNotEqual, Val: (*time.Time)(nil)},
		"Name":      {Op: filters.OpEqual, Val: "Alice"},
		"Score":     {Op: filters.OpGreater, Val: 1.5},
	}, filters.Raw(".Rate < ?", 5))

	wants := map[string]string{
		DialectPostgres: `SELECT COUNT(*) AS cnt FROM "test_struct_with_nullable_fields" WHERE ("age" IS NULL AND "birth_date" IS NOT NULL ` +
			`AND "name"=$1 AND "score">$2) AND ("rate" < $3);`,
		DialectSQLite: `SELECT COUNT(*) AS cnt FROM "test_struct_with_nullable_fields" WHERE ("age" IS NULL AND "birth_date" IS NOT NULL ` +
			`AND "name"=? AND "score">?) AND ("rate" < ?);`,
	}

	for dialect, want := range wants {
		t.Run(dialect, func(t *testing.T) {
			got, err := mustNew(t, &TestStructWithNullableFields{}, Options{Dialect: dialect}).SelectCount(f)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if got != want {
				t.Fatalf("\nwant %v\ngot  %v", want, got)
			}
		})
	}

	wantValues := []interface{}{"Alice", 1.5, 5}
	if got := filters.FiltersInterfaces(f); len(got) != 3 || got[0] != wantValues[0] || got[1] != wantValues[1] || got[2] != wantValues[2] {
		t.Fatalf("FiltersInterfaces returned %v, want %v", got, wantValues)
	}
}

func TestSQLInConditions(t *testing.T) {
	f := filters.And(filters.Filters{
		"Age":   {Op: filters.OpEqual, Val: nil},
		"Name":  {Op: filters.OpIn, Val: []string{"Alice", "Bob"}},
		"Rate":  {Op: filters.OpIn, Val: []float32{}},
		"Score": {Op: filters.OpGreater, Val: 1.5},
	}, filters.Raw(".ID IN (?)", []uint64{1, 2}))

	wants := map[string]string{
		DialectPostgres: `SELECT COUNT(*) AS cnt FROM "test_struct_with_nullable_fields" WHERE ("age" IS NULL AND "name" IN ($1,$2) ` +
			`AND 1=0 AND "score">$3) AND ("id" IN ($4,$5));`,
		DialectSQLite: `SELECT COUNT(*) AS cnt FROM "test_struct_with_nullable_fields" WHERE ("age" IS NULL AND "name" IN (?,?) ` +
			`AND 1=0 AND "score">?) AND ("id" IN (?,?));`,
	}

	for dialect, want := range wants {
		t.Run(dialect, func(t *testing.T) {
			h := mustNew(t, &TestStructWithNullableFields{}, Options{Dialect: dialect})
			got, err := h.SelectCount(f)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if got != want {
				t.Fatalf("\nwant %v\ngot  %v", want, got)
			}

			if _, err := h.SelectCount(&filters.Filters{"Name": {Op: filters.OpIn, Val: "Alice"}}); err == nil {
				t.Fatal("SelectCount did not return an err for an OpIn filter that is not a list")
			}
		})
	}

	wantValues := []interface{}{"Alice", "Bob", 1.5, uint64(1), uint64(2)}
	if got := filters.FiltersInterfaces(f); !reflect.DeepEqual(got, wantValues) {
		t.Fatalf("FiltersInterfaces returned %v, want %v", got, wantValues)
	}
}

type TestStructWithForeignKeys struct {
	ID         uint64
	CustomerID uint64  `sql:"fk:customer on_delete:cascade"`
	ShipperID  *uint64 `sql:"fk:shipper on_delete:set_null uniq"`
	NoteID     uint64  `sql:"fk:note"`
}

func TestSQLForeignKeys(t *testing.T) {
	wants := map[string][2]string{
		DialectPostgres: {
			`CREATE TABLE IF NOT EXISTS "app_test_struct_with_foreign_keys" ("id" SERIAL PRIMARY KEY,` +
				`"customer_id" BIGINT NOT NULL DEFAULT 0 REFERENCES "app_customer"("id") ON DELETE CASCADE,` +
				`"shipper_id" BIGINT UNIQUE REFERENCES "app_shipper"("id") ON DELETE SET NULL,` +
				`"note_id" BIGINT NOT NULL DEFAULT 0 REFERENCES "app_note"("id"));`,
			`ALTER TABLE "app_test_struct_with_foreign_keys" ADD COLUMN "shipper_id" BIGINT UNIQUE REFERENCES "app_shipper"("id") ON DELETE SET NULL`,
		},
		DialectSQLite: {
			`CREATE TABLE IF NOT EXISTS "app_test_struct_with_foreign_keys" ("id" INTEGER PRIMARY KEY,` +
				`"customer_id" INTEGER NOT NULL DEFAULT 0 REFERENCES "app_customer"("id") ON DELETE CASCADE,` +
				`"shipper_id" INTEGER UNIQUE REFERENCES "app_shipper"("id") ON DELETE SET NULL,` +
				`"note_id" INTEGER NOT NULL DEFAULT 0 REFERENCES "app_note"("id"));`,
			`ALTER TABLE "app_test_struct_with_foreign_keys" ADD COLUMN "shipper_id" INTEGER REFERENCES "app_shipper"("id") ON DELETE SET NULL`,
		},
	}

	for dialect, want := range wants {
		t.Run(dialect, func(t *testing.T) {
			h := mustNew(t, &TestStructWithForeignKeys{}, Options{Dialect: dialect, TableNamePrefix: "app_"})
			if got := h.CreateTable(); got != want[0] {
				t.Fatalf("\nwant %v\ngot  %v", want[0], got)
			}
			if got := h.Columns()[2].AddStatements[0]; got != want[1] {
				t.Fatalf("\nwant %v\ngot  %v", want[1], got)
			}
			if fk := h.Columns()[3].ForeignKey; fk == nil || fk.Table != "app_note" || fk.OnDelete != "NO ACTION" {
				t.Fatalf("Columns returned foreign key %+v, want app_note with NO ACTION", fk)
			}
		})
	}
}

type TestStructWithAllBuilders struct {
	ID   uint64
	Name string
}

func (s *TestStructWithAllBuilders) BuildDropTableQuery(p string) (string, error) {
	return "custom drop " + p, nil
}
func (s *TestStructWithAllBuilders) BuildCreateTableQuery(p string) (string, error) {
	return "custom create " + p, nil
}
func (s *TestStructWithAllBuilders) BuildDeleteByIDQuery(p string) (string, error) {
	return "custom delete by id " + p, nil
}
func (s *TestStructWithAllBuilders) BuildDeletePrefixQuery(p string) (string, error) {
	return "custom delete prefix " + p, nil
}
func (s *TestStructWithAllBuilders) BuildUpdateByIDQuery(p string) (string, error) {
	return "custom update by id " + p, nil
}
func (s *TestStructWithAllBuilders) BuildUpdatePrefixQuery(p string) (string, error) {
	return "custom update prefix " + p, nil
}
func (s *TestStructWithAllBuilders) BuildInsertQuery(p string) (string, error) {
	return "custom insert " + p, nil
}
func (s *TestStructWithAllBuilders) BuildInsertOnConflictUpdateQuery(p string) (string, error) {
	return "custom upsert " + p, nil
}
func (s *TestStructWithAllBuilders) BuildSelectByIDQuery(p string) (string, error) {
	return "custom select by id " + p, nil
}
func (s *TestStructWithAllBuilders) BuildSelectPrefixQuery(p string) (string, error) {
	return "custom select prefix " + p, nil
}
func (s *TestStructWithAllBuilders) BuildSelectCountPrefixQuery(p string) (string, error) {
	return "custom count prefix " + p, nil
}

type TestStructWithFailingBuilder struct {
	ID   uint64
	Name string
}

var errFailingBuilder = errors.New("failing builder")

func (s *TestStructWithFailingBuilder) BuildInsertQuery(string) (string, error) {
	return "", errFailingBuilder
}

func TestSQLCustomBuilders(t *testing.T) {
	for _, dialect := range []string{DialectPostgres, DialectSQLite} {
		t.Run(dialect, func(t *testing.T) {
			h := mustNew(t, &TestStructWithAllBuilders{}, Options{Dialect: dialect, TableNamePrefix: "p_"})

			selectQuery, _ := h.Select(nil, 0, 0, nil)
			selectCountQuery, _ := h.SelectCount(nil)
			deleteQuery, _ := h.Delete(nil)
			updateQuery, _ := h.Update(map[string]interface{}{"Name": "x"}, nil)

			placeholder := map[string]string{DialectPostgres: "$1", DialectSQLite: "?"}[dialect]
			tests := map[string][2]string{
				"DropTable":              {h.DropTable(), "custom drop p_;"},
				"CreateTable":            {h.CreateTable(), "custom create p_;"},
				"DeleteByID":             {h.DeleteByID(), "custom delete by id p_;"},
				"UpdateByID":             {h.UpdateByID(), "custom update by id p_;"},
				"Insert":                 {h.Insert(), "custom insert p_;"},
				"InsertOnConflictUpdate": {h.InsertOnConflictUpdate(), "custom upsert p_;"},
				"SelectByID":             {h.SelectByID(), "custom select by id p_;"},
				"Select":                 {selectQuery, "custom select prefix p_;"},
				"SelectCount":            {selectCountQuery, "custom count prefix p_;"},
				"Delete":                 {deleteQuery, "custom delete prefix p_;"},
				"Update":                 {updateQuery, `custom update prefix p_ "name"=` + placeholder + ";"},
			}
			for name, gotWant := range tests {
				if gotWant[0] != gotWant[1] {
					t.Fatalf("%s returned %q, want %q", name, gotWant[0], gotWant[1])
				}
			}

			if _, err := New(&TestStructWithFailingBuilder{}, Options{Dialect: dialect}); !errors.Is(err, errFailingBuilder) {
				t.Fatalf("New returned err %v, want %v", err, errFailingBuilder)
			}
		})
	}
}

func TestSQLiteDeleteQueries(t *testing.T) {
	h := mustNew(t, testStructObj, Options{Dialect: DialectSQLite})

	got, err := h.Delete(&filters.Filters{"Price": {Op: filters.OpEqual, Val: 444}, "Age": {Op: filters.OpIn, Val: []int{1, 2}}})
	want := `DELETE FROM "test_struct" WHERE "age" IN (?,?) AND "price"=?;`
	if err != nil || got != want {
		t.Fatalf("Delete returned %q, %v, want %q", got, err, want)
	}

	if _, err := h.Delete(&filters.Filters{"Price": {Op: filters.OpIn, Val: 444}}); err == nil {
		t.Fatal("Delete did not return an err for an invalid filter")
	}
}

func TestBuilderHelpers(t *testing.T) {
	h := mustNew(t, testStructObj, Options{})

	if got := h.DatabaseColumnToFieldName("post_code2"); got != "PostCode2" {
		t.Fatalf("DatabaseColumnToFieldName returned %q, want PostCode2", got)
	}
	if got := h.DatabaseColumnToFieldName("unknown"); got != "" {
		t.Fatalf("DatabaseColumnToFieldName returned %q for an unknown column, want an empty string", got)
	}

	if !IsStructField(testStructObj, "PostCode") || IsStructField(testStructObj, "Unknown") {
		t.Fatal("IsStructField returned wrong results")
	}

	want := "CREATE TABLE \"t\" (\n  \"id\" BIGINT,\n  \"name\" TEXT)"
	if got := PrettifyCreateTable(`CREATE TABLE "t" ("id" BIGINT,"name" TEXT)`); got != want {
		t.Fatalf("PrettifyCreateTable returned %q, want %q", got, want)
	}
}

func TestSQLOrderDirections(t *testing.T) {
	order := []string{"Age", "ASC", "Price", "DESC", "Flags", "desc", "Key", "Desc", "ID", "anything"}
	want := `ORDER BY "age" ASC,"price" DESC,"flags" DESC,"key" DESC,"id" ASC;`

	for _, dialect := range []string{DialectPostgres, DialectSQLite} {
		got, err := mustNew(t, testStructObj, Options{Dialect: dialect}).Select(order, 0, 0, nil)
		if err != nil || !strings.HasSuffix(got, want) {
			t.Fatalf("%s: Select returned %q, %v, want it to end with %q", dialect, got, err, want)
		}
	}
}
