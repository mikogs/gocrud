package gocrud

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

type Profile struct {
	ID        uint64
	Name      string   `validate:"required"`
	Nickname  *string  `validate:"omitempty,min=2" crud_regexp:"^[a-z]+$"`
	Age       *int64   `validate:"omitempty,min=0,max=150"`
	Score     *float64 `crud:"idx"`
	Verified  *bool
	BirthDate *time.Time
	Email     *string `crud:"uniq"`
}

func ptr[T any](v T) *T {
	return &v
}

func recreateProfileTable(t *testing.T, crud *CRUD) {
	t.Helper()

	_ = crud.DropTable(context.Background(), &Profile{})
	if err := crud.CreateTable(context.Background(), &Profile{}); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
}

func saveProfiles(t *testing.T, crud *CRUD) {
	t.Helper()

	profiles := []*Profile{
		{Name: "Alice", Nickname: ptr("ali"), Age: ptr(int64(30)), Score: ptr(10.5), Verified: ptr(true), Email: ptr("alice@example.com")},
		{Name: "Bob", Age: ptr(int64(25)), Score: ptr(9.0), Verified: ptr(false)},
		{Name: "Carol"},
	}
	for _, profile := range profiles {
		if err := crud.Save(context.Background(), profile, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}
}

func loadProfile(t *testing.T, crud *CRUD, id uint64) *Profile {
	t.Helper()

	profile := &Profile{}
	if err := crud.Load(context.Background(), profile, strconv.FormatUint(id, 10), LoadOptions{}).Error; err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}

	return profile
}

func fatalIfNotProfiles(t *testing.T, crud *CRUD, options GetOptions, want []string) {
	t.Helper()

	objs, _, err := crud.Get(context.Background(), func() interface{} { return &Profile{} }, options)
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}

	got := make([]string, 0, len(objs))
	for _, obj := range objs {
		got = append(got, obj.(*Profile).Name)
	}
	if len(got) != len(want) {
		t.Fatalf("Get returned %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Get returned %v, want %v", got, want)
		}
	}
}

func testNullableRoundTrip(t *testing.T, crud *CRUD) {
	recreateProfileTable(t, crud)
	ctx := context.Background()

	birthDate := time.Date(1990, 5, 1, 0, 0, 0, 0, mustLoadLocation(t, "Europe/Warsaw"))
	profile := &Profile{
		Name: "Alice", Nickname: ptr("ali"), Age: ptr(int64(0)), Score: ptr(0.0), Verified: ptr(false),
		BirthDate: &birthDate, Email: ptr("alice@example.com"),
	}
	if err := crud.Save(ctx, profile, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	loaded := loadProfile(t, crud, profile.ID)
	if loaded.Nickname == nil || *loaded.Nickname != "ali" || loaded.Age == nil || *loaded.Age != 0 ||
		loaded.Score == nil || *loaded.Score != 0 || loaded.Verified == nil || *loaded.Verified ||
		loaded.BirthDate == nil || !loaded.BirthDate.Equal(birthDate) || loaded.BirthDate.Location() != time.UTC ||
		loaded.Email == nil || *loaded.Email != "alice@example.com" {
		t.Fatalf("Load returned %+v, want the zero values to be kept apart from NULL", *loaded)
	}

	loaded.Nickname, loaded.Age, loaded.Score, loaded.Verified, loaded.BirthDate, loaded.Email = nil, nil, nil, nil, nil, nil
	if err := crud.Save(ctx, loaded, SaveOptions{}); err != nil {
		t.Fatalf("Save failed to update: %s", err.Error())
	}

	loaded = loadProfile(t, crud, profile.ID)
	if loaded.Nickname != nil || loaded.Age != nil || loaded.Score != nil || loaded.Verified != nil || loaded.BirthDate != nil || loaded.Email != nil {
		t.Fatalf("Load returned %+v, want NULL values", *loaded)
	}

	if _, err := crud.conn.ExecContext(ctx, `INSERT INTO profile (name) VALUES ('Imported')`); err != nil {
		t.Fatalf("ExecContext failed: %s", err.Error())
	}
	imported := loadProfile(t, crud, profile.ID+1)
	if imported.Name != "Imported" || imported.Nickname != nil || imported.BirthDate != nil {
		t.Fatalf("Load returned %+v, want NULL as the column default", *imported)
	}
}

func testNullableFilters(t *testing.T, crud *CRUD) {
	recreateProfileTable(t, crud)
	saveProfiles(t, crud)

	byName := []string{"Name", "asc"}
	tests := map[string]struct {
		filters sqlfilters.Filter
		want    []string
	}{
		"IS NULL":               {&sqlfilters.Filters{"Nickname": {Op: sqlfilters.OpEqual, Val: nil}}, []string{"Bob", "Carol"}},
		"IS NOT NULL":           {&sqlfilters.Filters{"Nickname": {Op: sqlfilters.OpNotEqual, Val: nil}}, []string{"Alice"}},
		"typed nil":             {&sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: (*int64)(nil)}}, []string{"Carol"}},
		"value":                 {&sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: int64(25)}}, []string{"Bob"}},
		"pointer":               {&sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: ptr(int64(25))}}, []string{"Bob"}},
		"bool false":            {&sqlfilters.Filters{"Verified": {Op: sqlfilters.OpEqual, Val: false}}, []string{"Bob"}},
		"float":                 {&sqlfilters.Filters{"Score": {Op: sqlfilters.OpGreater, Val: 9.5}}, []string{"Alice"}},
		"comparison skips NULL": {&sqlfilters.Filters{"Age": {Op: sqlfilters.OpLower, Val: int64(100)}}, []string{"Alice", "Bob"}},
		"NULL and values": {&sqlfilters.Filters{
			"Email":    {Op: sqlfilters.OpEqual, Val: nil},
			"Name":     {Op: sqlfilters.OpNotEqual, Val: "Carol"},
			"Verified": {Op: sqlfilters.OpNotEqual, Val: nil},
			"Score":    {Op: sqlfilters.OpLower, Val: 100.0},
		}, []string{"Bob"}},
		"raw": {sqlfilters.Raw(".BirthDate IS NULL AND .Age > ?", int64(26)), []string{"Alice"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fatalIfNotProfiles(t, crud, GetOptions{Order: byName, Filters: test.filters}, test.want)

			cnt, err := crud.GetCount(context.Background(), &Profile{}, GetCountOptions{Filters: test.filters})
			if err != nil || cnt != uint64(len(test.want)) {
				t.Fatalf("GetCount returned %d, %v, want %d", cnt, err, len(test.want))
			}
		})
	}
}

func testNullableStringFilters(t *testing.T, crud *CRUD) {
	recreateProfileTable(t, crud)
	saveProfiles(t, crud)

	tests := map[string]struct {
		filters sqlfilters.Filters
		want    []string
	}{
		"empty is NULL":      {sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: ""}}, []string{"Carol"}},
		"empty is NOT NULL":  {sqlfilters.Filters{"Nickname": {Op: sqlfilters.OpNotEqual, Val: ""}}, []string{"Alice"}},
		"value":              {sqlfilters.Filters{"Age": {Op: sqlfilters.OpGreater, Val: "26"}}, []string{"Alice"}},
		"bool":               {sqlfilters.Filters{"Verified": {Op: sqlfilters.OpEqual, Val: "true"}}, []string{"Alice"}},
		"nil is not changed": {sqlfilters.Filters{"BirthDate": {Op: sqlfilters.OpEqual, Val: nil}}, []string{"Alice", "Bob", "Carol"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fatalIfNotProfiles(t, crud, GetOptions{
				Order:                    []string{"Name", "asc"},
				Filters:                  &test.filters,
				ConvertFiltersFromString: true,
			}, test.want)
		})
	}

	_, _, err := crud.Get(context.Background(), func() interface{} { return &Profile{} }, GetOptions{
		Filters:                  &sqlfilters.Filters{"Age": {Op: sqlfilters.OpEqual, Val: "old"}},
		ConvertFiltersFromString: true,
	})
	fatalIfNotViolations(t, err, map[string]string{"Age": ViolationType})
}

func testNullableInvalidFilters(t *testing.T, crud *CRUD) {
	recreateProfileTable(t, crud)

	tests := map[string]sqlfilters.Filters{
		"NULL on a field that is not nullable": {"Name": {Op: sqlfilters.OpEqual, Val: nil}},
		"nil with another operator":            {"Age": {Op: sqlfilters.OpGreater, Val: nil}},
	}

	for name, filters := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := crud.Get(context.Background(), func() interface{} { return &Profile{} }, GetOptions{Filters: &filters})
			for field := range filters {
				fatalIfNotViolations(t, err, map[string]string{field: ViolationType})
			}
		})
	}
}

func testNullableRawFilterWithStringFilters(t *testing.T, crud *CRUD) {
	recreateProfileTable(t, crud)
	saveProfiles(t, crud)

	fatalIfNotProfiles(t, crud, GetOptions{
		Order: []string{"Name", "asc"},
		Filters: sqlfilters.And(
			sqlfilters.Filters{"Verified": {Op: sqlfilters.OpNotEqual, Val: ""}},
			sqlfilters.Raw(".Name != ?", "Alice"),
		),
		ConvertFiltersFromString: true,
	}, []string{"Bob"})
}

func testNullableUpdateMultiple(t *testing.T, crud *CRUD) {
	recreateProfileTable(t, crud)
	saveProfiles(t, crud)
	ctx := context.Background()

	err := crud.UpdateMultiple(ctx, &Profile{}, map[string]interface{}{"Nickname": "bobby", "Age": nil}, UpdateMultipleOptions{
		Filters: &sqlfilters.Filters{"Name": {Op: sqlfilters.OpEqual, Val: "Bob"}},
	})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	bob := loadProfile(t, crud, 2)
	if bob.Nickname == nil || *bob.Nickname != "bobby" || bob.Age != nil {
		t.Fatalf("UpdateMultiple set %+v, want Nickname bobby and Age NULL", *bob)
	}

	err = crud.UpdateMultiple(ctx, &Profile{}, map[string]interface{}{"Nickname": "", "Score": "1.5"}, UpdateMultipleOptions{
		ConvertValuesFromString: true,
		Filters:                 &sqlfilters.Filters{"Verified": {Op: sqlfilters.OpEqual, Val: nil}},
	})
	if err != nil {
		t.Fatalf("UpdateMultiple failed to update from strings: %s", err.Error())
	}
	carol := loadProfile(t, crud, 3)
	if carol.Nickname != nil || carol.Score == nil || *carol.Score != 1.5 {
		t.Fatalf("UpdateMultiple set %+v, want Nickname NULL and Score 1.5", *carol)
	}

	err = crud.UpdateMultiple(ctx, &Profile{}, map[string]interface{}{"Name": nil, "Nickname": "x"}, UpdateMultipleOptions{})
	fatalIfNotViolations(t, err, map[string]string{"Name": ViolationType, "Nickname": "min"})
}

func testNullableValidation(t *testing.T, crud *CRUD) {
	recreateProfileTable(t, crud)

	tests := map[string]struct {
		profile *Profile
		want    map[string]string
	}{
		"too short":    {&Profile{Name: "Alice", Nickname: ptr("a")}, map[string]string{"Nickname": "min"}},
		"regexp":       {&Profile{Name: "Alice", Nickname: ptr("Ali")}, map[string]string{"Nickname": ViolationRegExp}},
		"out of range": {&Profile{Name: "Alice", Age: ptr(int64(200))}, map[string]string{"Age": "max"}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fatalIfNotViolations(t, crud.Save(context.Background(), test.profile, SaveOptions{}), test.want)
		})
	}

	if err := crud.Save(context.Background(), &Profile{Name: "Alice"}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed for NULL values: %s", err.Error())
	}
}

func testNullableGetCountOnUniq(t *testing.T, crud *CRUD) {
	uniqCRUD := New(crud.db, Options{Dialect: crud.dialect, Flags: GetCountOnUniq})
	recreateProfileTable(t, uniqCRUD)
	ctx := context.Background()

	for _, name := range []string{"Alice", "Bob"} {
		if err := uniqCRUD.Save(ctx, &Profile{Name: name}, SaveOptions{}); err != nil {
			t.Fatalf("Save failed for a second NULL in a unique field: %s", err.Error())
		}
	}

	if err := uniqCRUD.Save(ctx, &Profile{Name: "Carol", Email: ptr("carol@example.com")}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	err := uniqCRUD.Save(ctx, &Profile{Name: "Dave", Email: ptr("carol@example.com")}, SaveOptions{})

	var uniqErr *UniqError
	if !errors.As(err, &uniqErr) || uniqErr.Field != "Email" {
		t.Fatalf("Save returned err %v, want an uniq error for Email", err)
	}
}

func TestNullable_RoundTrip(t *testing.T)        { testNullableRoundTrip(t, testCRUD) }
func TestSQLite_Nullable_RoundTrip(t *testing.T) { testNullableRoundTrip(t, testCRUDSQLite) }

func TestNullable_Filters(t *testing.T)        { testNullableFilters(t, testCRUD) }
func TestSQLite_Nullable_Filters(t *testing.T) { testNullableFilters(t, testCRUDSQLite) }

func TestNullable_StringFilters(t *testing.T)        { testNullableStringFilters(t, testCRUD) }
func TestSQLite_Nullable_StringFilters(t *testing.T) { testNullableStringFilters(t, testCRUDSQLite) }

func TestNullable_InvalidFilters(t *testing.T)        { testNullableInvalidFilters(t, testCRUD) }
func TestSQLite_Nullable_InvalidFilters(t *testing.T) { testNullableInvalidFilters(t, testCRUDSQLite) }

func TestNullable_RawFilterWithStringFilters(t *testing.T) {
	testNullableRawFilterWithStringFilters(t, testCRUD)
}
func TestSQLite_Nullable_RawFilterWithStringFilters(t *testing.T) {
	testNullableRawFilterWithStringFilters(t, testCRUDSQLite)
}

func TestNullable_UpdateMultiple(t *testing.T)        { testNullableUpdateMultiple(t, testCRUD) }
func TestSQLite_Nullable_UpdateMultiple(t *testing.T) { testNullableUpdateMultiple(t, testCRUDSQLite) }

func TestNullable_Validation(t *testing.T)        { testNullableValidation(t, testCRUD) }
func TestSQLite_Nullable_Validation(t *testing.T) { testNullableValidation(t, testCRUDSQLite) }

func TestNullable_GetCountOnUniq(t *testing.T)        { testNullableGetCountOnUniq(t, testCRUD) }
func TestSQLite_Nullable_GetCountOnUniq(t *testing.T) { testNullableGetCountOnUniq(t, testCRUDSQLite) }
