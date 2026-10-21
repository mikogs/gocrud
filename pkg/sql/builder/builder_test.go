package builder

import (
	"errors"
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

	got, err = h.Select([]string{"EmailSecondary", "asc", "Age", "desc"}, 1, 3, &filters.Filters{
		"Price":     {Op: filters.OpEqual, Val: 33},
		"PostCode2": {Op: filters.OpEqual, Val: "11-111"},
		filters.Raw: {
			Op: filters.OpOR,
			Val: []interface{}{
				".Price=? OR (.EmailSecondary=? AND .Age IN (?)) OR (.Age IN (?)) OR (.EmailSecondary IN (?))",
				// We do not really care about the values, the query contains $x only symbols
				// However, we need to pass either value or an array so that an array can be extracted into multiple $x's
				0,
				0,
				[]int{0, 0, 0, 0},
				[]int{0, 0, 0},
				[]int{0, 0},
			},
		},
	})
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
		&filters.Filters{
			"Price": {Op: filters.OpEqual, Val: 4444},
			filters.Raw: {
				Op: filters.OpAND,
				Val: []interface{}{
					".Price=? OR .EmailSecondary=? OR .Age IN (?)",
					0,
					0,
					[]int{0, 0, 0},
				},
			},
		})
	want = `DELETE FROM "test_struct" WHERE ("price"=$1) AND ` +
		`("price"=$2 OR "email_secondary"=$3 OR "age" IN ($4,$5,$6));`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}

	got, err = h.Delete(
		&filters.Filters{
			filters.Raw: {
				Val: []interface{}{
					".Price=? OR .EmailSecondary=? OR .Age IN (?)",
					0,
					0,
					[]int{0, 0, 0},
				},
			},
		})
	want = `DELETE FROM "test_struct" WHERE ("price"=$1 OR "email_secondary"=$2 OR "age" IN ($3,$4,$5));`
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("\nwant %v\ngot  %v", want, got)
	}

	got, err = h.DeleteReturningID(
		&filters.Filters{
			filters.Raw: {
				Val: []interface{}{
					".Price=? OR .EmailSecondary=? OR .Age IN (?)",
					0,
					0,
					[]int{0, 0, 0},
				},
			},
		})
	want = `DELETE FROM "test_struct" WHERE ("price"=$1 OR "email_secondary"=$2 OR "age" IN ($3,$4,$5)) RETURNING "id";`
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

var aliasesTestFilters = &filters.Filters{
	"Name":      {Op: filters.OpLike, Val: "a%"},
	"Price":     {Op: filters.OpGreater, Val: 1},
	filters.Raw: {Op: filters.OpAND, Val: []interface{}{".Price < ?", 100}},
}

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
	tests := map[string]interface{}{
		"not a slice":                 ".Price > ?",
		"slice of strings":            []string{".Price > ?"},
		"query is not a string":       []interface{}{1, 2},
		"query is nil":                []interface{}{nil},
		"slice of strings with value": []string{".Price > ?", "1"},
	}

	for _, dialect := range []string{DialectPostgres, DialectSQLite} {
		h := mustNew(t, testStructObj, Options{Dialect: dialect})

		for name, rawValue := range tests {
			t.Run(dialect+"/"+name, func(t *testing.T) {
				f := &filters.Filters{filters.Raw: {Op: filters.OpAND, Val: rawValue}}

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
	f := &filters.Filters{filters.Raw: {Op: filters.OpAND, Val: []interface{}{".Price = ? OR .Age = ?", nil, 5}}}

	wants := map[string]string{
		DialectPostgres: `SELECT COUNT(*) AS cnt FROM "test_struct" WHERE ("price" = $1 OR "age" = $2);`,
		DialectSQLite:   `SELECT COUNT(*) AS cnt FROM "test_struct" WHERE ("price" = ? OR "age" = ?);`,
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
