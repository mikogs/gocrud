package registry

import (
	"context"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/gocrudtest"
)

func TestNum(t *testing.T) {
	recreateTestStructTable()

	for i := 1; i < 51; i++ {
		ts := gocrudtest.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	for i := 1; i < 151; i++ {
		ts := gocrudtest.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	num, err := testService.Num(context.Background(), "teststruct", Filters{Vals: map[string]string{
		"Price":        "444",
		"PrimaryEmail": "primary@example.com",
	}, Ops: map[string]string{
		"Price":        "eq",
		"PrimaryEmail": "eq",
	}})
	if err != nil {
		t.Fatalf("Num failed to return list of objects: %s", err.Error())
	}
	if num != 150 {
		t.Fatalf("Num failed to return list of objects, want %v, got %v", 150, num)
	}
}

func TestNum_InvalidFilterOp(t *testing.T) {
	_, err := testService.Num(context.Background(), "teststruct",
		Filters{Vals: map[string]string{"Price": "444"}, Ops: map[string]string{"Price": "bogus"}})
	fatalIfNotFilterViolations(t, err, map[string]string{"Price": gocrud.ViolationType})
}
