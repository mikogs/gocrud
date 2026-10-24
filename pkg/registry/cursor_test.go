package registry

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/internal/fixtures"
)

func TestList_After(t *testing.T) {
	ctx := context.Background()
	recreateTestStructTable()
	for i := 0; i < 12; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		ts.Age = 1 + i%3
		ts.Key += string(rune('a' + i))
		if err := testCRUD.Save(ctx, ts, gocrud.SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	all, _, err := testService.List(ctx, "teststruct", ListOptions{Order: "Age", OrderDirection: "desc", Limit: 100})
	if err != nil {
		t.Fatalf("List failed: %s", err.Error())
	}
	var want, got []uint64
	for _, obj := range all {
		want = append(want, obj.(*fixtures.TestStruct).ID)
	}

	after := ""
	for page := 0; page < 10; page++ {
		objs, _, err := testService.List(ctx, "teststruct", ListOptions{Order: "Age", OrderDirection: "desc", Limit: 5, After: after})
		if err != nil {
			t.Fatalf("List of the page %d failed: %s", page, err.Error())
		}
		for _, obj := range objs {
			got = append(got, obj.(*fixtures.TestStruct).ID)
		}
		if len(objs) < 5 {
			break
		}
		if after, err = testService.NextCursor(objs[len(objs)-1], "Age", "desc"); err != nil {
			t.Fatalf("NextCursor failed: %s", err.Error())
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the pages returned %v, want %v", got, want)
	}

	_, _, err = testService.List(ctx, "teststruct", ListOptions{Order: "Age", OrderDirection: "asc", Limit: 5, After: after})
	var filterErr *FilterValidationError
	if !errors.As(err, &filterErr) || filterErr.Violations["after"] != gocrud.ViolationType || !errors.Is(err, gocrud.ErrCursor) {
		t.Fatalf("List with a cursor of another order returned %v, want a FilterValidationError of after", err)
	}
}
