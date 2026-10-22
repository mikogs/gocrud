package gocrud

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

type Event struct {
	ID       uint64
	Name     string    `validate:"required"`
	StartsAt time.Time `validate:"required" crud:"idx"`
	EndsAt   time.Time
}

type Booking struct {
	ID       uint64
	Room     string
	StartsAt time.Time `crud:"uniq"`
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()

	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation failed: %s", err.Error())
	}

	return location
}

func recreateEventTable(t *testing.T, crud *CRUD) {
	t.Helper()

	_ = crud.DropTable(context.Background(), &Event{})
	if err := crud.CreateTable(context.Background(), &Event{}); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
}

// saveEvents saves events starting at 10:00, 11:00 and 12:00 UTC, each one in a different time zone.
// When the times were compared in their own time zones, or as strings with different offsets,
// they would be in a different order: 19:00 (Tokyo), 07:00 (New York), 12:00 (UTC).
func saveEvents(t *testing.T, crud *CRUD) {
	t.Helper()

	startsAt := []time.Time{
		time.Date(2024, 5, 1, 19, 0, 0, 0, mustLoadLocation(t, "Asia/Tokyo")),
		time.Date(2024, 5, 1, 7, 0, 0, 0, mustLoadLocation(t, "America/New_York")),
		time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
	}
	for i, at := range startsAt {
		event := &Event{Name: "Event " + strconv.Itoa(i+1), StartsAt: at}
		if err := crud.Save(context.Background(), event, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}
}

func fatalIfNotEvents(t *testing.T, crud *CRUD, options GetOptions, want []string) {
	t.Helper()

	objs, _, err := crud.Get(context.Background(), func() interface{} { return &Event{} }, options)
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}

	got := make([]string, 0, len(objs))
	for _, obj := range objs {
		got = append(got, obj.(*Event).Name)
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

func testTimeRoundTrip(t *testing.T, crud *CRUD, precision time.Duration) {
	recreateEventTable(t, crud)
	ctx := context.Background()

	startsAt := time.Date(2024, 5, 1, 12, 30, 15, 123456789, mustLoadLocation(t, "Europe/Warsaw"))
	event := &Event{Name: "Conference", StartsAt: startsAt}
	if err := crud.Save(ctx, event, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	loaded := &Event{}
	if _, err := crud.Load(ctx, loaded, event.ID, LoadOptions{}); err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if !loaded.StartsAt.Equal(startsAt.Round(precision)) {
		t.Fatalf("Load returned StartsAt %v, want %v", loaded.StartsAt, startsAt.Round(precision))
	}
	if !loaded.EndsAt.IsZero() {
		t.Fatalf("Load returned EndsAt %v, want the zero time", loaded.EndsAt)
	}

	endsAt := startsAt.Add(2 * time.Hour).Round(precision)
	loaded.EndsAt = endsAt
	if err := crud.Save(ctx, loaded, SaveOptions{}); err != nil {
		t.Fatalf("Save failed to update: %s", err.Error())
	}

	objs, _, err := crud.Get(ctx, func() interface{} { return &Event{} }, GetOptions{})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if len(objs) != 1 || !objs[0].(*Event).EndsAt.Equal(endsAt) {
		t.Fatalf("Get returned %+v, want EndsAt %v", objs, endsAt)
	}

	if err := crud.Delete(ctx, loaded, DeleteOptions{}); err != nil {
		t.Fatalf("Delete failed: %s", err.Error())
	}
	if !loaded.StartsAt.IsZero() || !loaded.EndsAt.IsZero() {
		t.Fatal("Delete did not zero the time fields")
	}
}

func testTimeColumnDefault(t *testing.T, crud *CRUD) {
	recreateEventTable(t, crud)

	if _, err := crud.conn.ExecContext(context.Background(), `INSERT INTO event (name) VALUES ('Imported')`); err != nil {
		t.Fatalf("ExecContext failed: %s", err.Error())
	}

	loaded := &Event{}
	if _, err := crud.Load(context.Background(), loaded, 1, LoadOptions{}); err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if loaded.Name != "Imported" || !loaded.StartsAt.IsZero() || !loaded.EndsAt.IsZero() {
		t.Fatalf("Load returned %+v, want zero times", *loaded)
	}
}

func testTimeFiltersAndOrder(t *testing.T, crud *CRUD) {
	recreateEventTable(t, crud)
	saveEvents(t, crud)

	fatalIfNotEvents(t, crud, GetOptions{Order: []string{"StartsAt", "asc"}},
		[]string{"Event 1", "Event 2", "Event 3"})
	fatalIfNotEvents(t, crud, GetOptions{Order: []string{"StartsAt", "desc"}},
		[]string{"Event 3", "Event 2", "Event 1"})

	tenThirtyUTC := time.Date(2024, 5, 1, 12, 30, 0, 0, mustLoadLocation(t, "Europe/Warsaw"))
	fatalIfNotEvents(t, crud, GetOptions{
		Order:   []string{"StartsAt", "asc"},
		Filters: &sqlfilters.Filters{"StartsAt": {Op: sqlfilters.OpGreater, Val: tenThirtyUTC}},
	}, []string{"Event 2", "Event 3"})

	elevenUTC := time.Date(2024, 5, 1, 11, 0, 0, 0, time.UTC)
	fatalIfNotEvents(t, crud, GetOptions{
		Filters: &sqlfilters.Filters{"StartsAt": {Op: sqlfilters.OpEqual, Val: elevenUTC.In(mustLoadLocation(t, "Asia/Tokyo"))}},
	}, []string{"Event 2"})

	cnt, err := crud.GetCount(context.Background(), &Event{}, GetCountOptions{
		Filters: sqlfilters.Raw(".StartsAt >= ? AND .StartsAt < ?", elevenUTC, &tenThirtyUTC),
	})
	if err != nil || cnt != 0 {
		t.Fatalf("GetCount returned %d, %v, want 0", cnt, err)
	}
}

func testTimeStringFilters(t *testing.T, crud *CRUD) {
	recreateEventTable(t, crud)
	saveEvents(t, crud)

	tests := map[string]struct {
		op    int
		value string
		want  []string
	}{
		"RFC 3339 with offset": {sqlfilters.OpGreaterOrEqual, "2024-05-01T13:00:00+02:00", []string{"Event 2", "Event 3"}},
		"RFC 3339 in UTC":      {sqlfilters.OpEqual, "2024-05-01T12:00:00Z", []string{"Event 3"}},
		"datetime-local":       {sqlfilters.OpLower, "2024-05-01T10:30", []string{"Event 1"}},
		"date":                 {sqlfilters.OpLower, "2024-05-02", []string{"Event 1", "Event 2", "Event 3"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fatalIfNotEvents(t, crud, GetOptions{
				Order:                    []string{"StartsAt", "asc"},
				Filters:                  &sqlfilters.Filters{"StartsAt": {Op: test.op, Val: test.value}},
				ConvertFiltersFromString: true,
			}, test.want)
		})
	}

	_, _, err := crud.Get(context.Background(), func() interface{} { return &Event{} }, GetOptions{
		Filters:                  &sqlfilters.Filters{"StartsAt": {Op: sqlfilters.OpEqual, Val: "yesterday"}},
		ConvertFiltersFromString: true,
	})
	fatalIfNotViolations(t, err, map[string]string{"StartsAt": ViolationType})
}

func testTimeUpdateMultiple(t *testing.T, crud *CRUD) {
	recreateEventTable(t, crud)
	saveEvents(t, crud)

	endsAt := time.Date(2024, 5, 1, 18, 0, 0, 0, time.UTC)
	err := crud.UpdateMultiple(context.Background(), &Event{}, map[string]interface{}{"EndsAt": endsAt},
		UpdateMultipleOptions{
			Filters: &sqlfilters.Filters{"StartsAt": {Op: sqlfilters.OpGreater, Val: time.Date(2024, 5, 1, 10, 30, 0, 0, time.UTC)}},
		})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}

	fatalIfNotEvents(t, crud, GetOptions{
		Order:   []string{"StartsAt", "asc"},
		Filters: &sqlfilters.Filters{"EndsAt": {Op: sqlfilters.OpEqual, Val: endsAt}},
	}, []string{"Event 2", "Event 3"})

	err = crud.UpdateMultiple(context.Background(), &Event{}, map[string]interface{}{"EndsAt": "2024-05-01T20:00"},
		UpdateMultipleOptions{ConvertValuesFromString: true})
	if err != nil {
		t.Fatalf("UpdateMultiple failed to update from a string: %s", err.Error())
	}
	fatalIfNotEvents(t, crud, GetOptions{
		Filters: &sqlfilters.Filters{"EndsAt": {Op: sqlfilters.OpEqual, Val: time.Date(2024, 5, 1, 20, 0, 0, 0, time.UTC)}},
	}, []string{"Event 1", "Event 2", "Event 3"})
}

func testTimeValidation(t *testing.T, crud *CRUD) {
	recreateEventTable(t, crud)

	err := crud.Save(context.Background(), &Event{Name: "No date"}, SaveOptions{})
	fatalIfNotViolations(t, err, map[string]string{"StartsAt": "required"})
}

func testTimeGetCountOnUniq(t *testing.T, crud *CRUD) {
	uniqCRUD := New(crud.db, Options{Dialect: crud.dialect, Flags: GetCountOnUniq})
	_ = uniqCRUD.DropTable(context.Background(), &Booking{})
	_ = uniqCRUD.CreateTable(context.Background(), &Booking{})

	startsAt := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := uniqCRUD.Save(context.Background(), &Booking{Room: "A", StartsAt: startsAt}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	sameInstant := &Booking{Room: "B", StartsAt: startsAt.In(mustLoadLocation(t, "America/New_York"))}
	err := uniqCRUD.Save(context.Background(), sameInstant, SaveOptions{})

	var uniqErr *UniqError
	if !errors.As(err, &uniqErr) || uniqErr.Field != "StartsAt" {
		t.Fatalf("Save returned err %v, want an uniq error for StartsAt", err)
	}
}

func TestTime_RoundTrip(t *testing.T)        { testTimeRoundTrip(t, testCRUD, time.Microsecond) }
func TestSQLite_Time_RoundTrip(t *testing.T) { testTimeRoundTrip(t, testCRUDSQLite, time.Nanosecond) }

func TestTime_ColumnDefault(t *testing.T)        { testTimeColumnDefault(t, testCRUD) }
func TestSQLite_Time_ColumnDefault(t *testing.T) { testTimeColumnDefault(t, testCRUDSQLite) }

func TestTime_FiltersAndOrder(t *testing.T)        { testTimeFiltersAndOrder(t, testCRUD) }
func TestSQLite_Time_FiltersAndOrder(t *testing.T) { testTimeFiltersAndOrder(t, testCRUDSQLite) }

func TestTime_StringFilters(t *testing.T)        { testTimeStringFilters(t, testCRUD) }
func TestSQLite_Time_StringFilters(t *testing.T) { testTimeStringFilters(t, testCRUDSQLite) }

func TestTime_UpdateMultiple(t *testing.T)        { testTimeUpdateMultiple(t, testCRUD) }
func TestSQLite_Time_UpdateMultiple(t *testing.T) { testTimeUpdateMultiple(t, testCRUDSQLite) }

func TestTime_Validation(t *testing.T)        { testTimeValidation(t, testCRUD) }
func TestSQLite_Time_Validation(t *testing.T) { testTimeValidation(t, testCRUDSQLite) }

func TestTime_GetCountOnUniq(t *testing.T)        { testTimeGetCountOnUniq(t, testCRUD) }
func TestSQLite_Time_GetCountOnUniq(t *testing.T) { testTimeGetCountOnUniq(t, testCRUDSQLite) }

func testTimeLoadedInUTC(t *testing.T, crud *CRUD) {
	recreateEventTable(t, crud)
	saveEvents(t, crud)

	loaded := &Event{}
	if _, err := crud.Load(context.Background(), loaded, 1, LoadOptions{}); err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if loaded.StartsAt.Location() != time.UTC || loaded.EndsAt.Location() != time.UTC {
		t.Fatalf("Load returned times in %v and %v, want UTC", loaded.StartsAt.Location(), loaded.EndsAt.Location())
	}

	objs, _, err := crud.Get(context.Background(), func() interface{} { return &Event{} }, GetOptions{})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	for _, obj := range objs {
		if obj.(*Event).StartsAt.Location() != time.UTC {
			t.Fatalf("Get returned a time in %v, want UTC", obj.(*Event).StartsAt.Location())
		}
	}
}

func TestTime_LoadedInUTC(t *testing.T)        { testTimeLoadedInUTC(t, testCRUD) }
func TestSQLite_Time_LoadedInUTC(t *testing.T) { testTimeLoadedInUTC(t, testCRUDSQLite) }

func testTimeValueInOwnSQL(t *testing.T, crud *CRUD) {
	recreateEventTable(t, crud)
	saveEvents(t, crud)

	err := crud.RunInTx(context.Background(), func(tx *CRUD) error {
		elevenUTC := time.Date(2024, 5, 1, 13, 0, 0, 0, mustLoadLocation(t, "Europe/Warsaw"))
		_, err := tx.Tx().ExecContext(context.Background(),
			`UPDATE event SET ends_at = $1 WHERE starts_at >= $2`,
			tx.TimeValue(elevenUTC.Add(time.Hour)), tx.TimeValue(elevenUTC))
		return err
	})
	if err != nil {
		t.Fatalf("RunInTx failed: %s", err.Error())
	}

	fatalIfNotEvents(t, crud, GetOptions{
		Order:   []string{"StartsAt", "asc"},
		Filters: &sqlfilters.Filters{"EndsAt": {Op: sqlfilters.OpEqual, Val: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)}},
	}, []string{"Event 2", "Event 3"})
}

func TestTime_ValueInOwnSQL(t *testing.T)        { testTimeValueInOwnSQL(t, testCRUD) }
func TestSQLite_Time_ValueInOwnSQL(t *testing.T) { testTimeValueInOwnSQL(t, testCRUDSQLite) }
