package service

import (
	"context"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/test"
)

func TestNum(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should be ignored by GetCount later on
	for i := 1; i < 51; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	// Insert data that should be selected by filters
	for i := 1; i < 151; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.Age = 30
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	// Get the data from the database
	num, err := testService.Num(context.Background(), "teststruct", map[string]string{
		"Price":        "444",
		"PrimaryEmail": "primary@example.com",
	}, map[string]string{
		"Price":        "eq",
		"PrimaryEmail": "eq",
	})
	if err != nil {
		t.Fatalf("Num failed to return list of objects: %s", err.Error())
	}
	if num != 150 {
		t.Fatalf("Num failed to return list of objects, want %v, got %v", 150, num)
	}
}

func TestNum_InvalidFilterOp(t *testing.T) {
	_, err := testService.Num(context.Background(), "teststruct",
		map[string]string{"Price": "444"},
		map[string]string{"Price": "bogus"})
	fatalIfNotFilterViolations(t, err, map[string]string{"Price": gocrud.ViolationType})
}
