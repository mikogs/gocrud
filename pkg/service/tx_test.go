package service

import (
	"context"
	"errors"
	"testing"

	"github.com/mikogs/gocrud/pkg/test"
)

func TestRunInTx(t *testing.T) {
	recreateTestStructTable()
	ctx := context.Background()

	err := testService.RunInTx(ctx, func(tx *CRUD) error {
		for i := 0; i < 2; i++ {
			if err := tx.Save(ctx, test.TestStructWithData(), 311); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunInTx failed: %s", err.Error())
	}

	errAbort := errors.New("abort")
	err = testService.RunInTx(ctx, func(tx *CRUD) error {
		if err := tx.Save(ctx, test.TestStructWithData(), 311); err != nil {
			return err
		}

		num, err := tx.Num(ctx, "teststruct", nil, nil)
		if err != nil {
			return err
		}
		if num != 3 {
			t.Fatalf("Num returned %d in the transaction, want 3", num)
		}
		return errAbort
	})
	if !errors.Is(err, errAbort) {
		t.Fatalf("RunInTx returned err %v, want %v", err, errAbort)
	}

	num, _ := testService.Num(ctx, "teststruct", nil, nil)
	if num != 2 {
		t.Fatalf("Num returned %d, want 2", num)
	}
}

func TestWithTx(t *testing.T) {
	recreateTestStructTable()
	ctx := context.Background()

	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx failed: %s", err.Error())
	}

	txService := testService.WithTx(tx)
	if txService.Tx() != tx || testService.Tx() != nil {
		t.Fatal("WithTx did not return a service running in the transaction only")
	}
	if err := txService.Save(ctx, test.TestStructWithData(), 311); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %s", err.Error())
	}

	num, _ := testService.Num(ctx, "teststruct", nil, nil)
	if num != 0 {
		t.Fatalf("Num returned %d after a rollback, want 0", num)
	}
}
