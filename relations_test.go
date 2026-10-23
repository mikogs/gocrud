package gocrud

import (
	"context"
	"errors"
	"testing"

	"github.com/mikogs/gocrud/internal/structinfo"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type RelCustomer struct {
	ID     uint64
	Name   string     `validate:"required"`
	Orders []RelOrder `crud:"rel:CustomerID"`
}

type RelOrder struct {
	ID         uint64
	CustomerID uint64       `crud:"idx"`
	Status     string       `validate:"required,oneof=placed paid"`
	Customer   *RelCustomer `crud:"rel:CustomerID"`
	Items      []*RelItem   `crud:"rel:OrderID"`
}

type RelItem struct {
	ID       uint64
	OrderID  uint64
	Quantity int64 `validate:"min=1"`
}

type RelInvalid struct {
	ID       uint64
	Name     string
	Customer *RelCustomer `crud:"rel:CustomerID"`
}

func testRelationFields(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[RelOrder](ctx, crud)
	if err := CreateTable[RelOrder](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	// related objects are not valid, but they are not validated with the order
	order := &RelOrder{
		CustomerID: 7,
		Status:     "placed",
		Customer:   &RelCustomer{ID: 7},
		Items:      []*RelItem{{Quantity: 0}},
	}
	if err := crud.Save(ctx, order, SaveOptions{}); err != nil {
		t.Fatalf("Save failed for an order with related objects: %s", err.Error())
	}
	if order.Customer == nil || len(order.Items) != 1 {
		t.Fatal("Save changed the related objects")
	}

	err := crud.Save(ctx, &RelOrder{CustomerID: 7, Status: "lost", Customer: &RelCustomer{ID: 7, Name: "Acme"}}, SaveOptions{})
	fatalIfNotViolations(t, err, map[string]string{"Status": "oneof"})

	loaded, err := Load[RelOrder](ctx, crud, order.ID)
	if err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if loaded.CustomerID != 7 || loaded.Status != "placed" || loaded.Customer != nil || loaded.Items != nil {
		t.Fatalf("Load returned %+v, want the columns only", *loaded)
	}

	err = UpdateMultiple[RelOrder](ctx, crud, map[string]interface{}{"Status": "paid"}, UpdateMultipleOptions{
		Filters: &sqlfilters.Filters{"CustomerID": {Op: sqlfilters.OpIn, Val: []uint64{7}}},
	})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}

	orders, err := Get[RelOrder](ctx, crud, GetOptions{
		Filters: &sqlfilters.Filters{"Status": {Op: sqlfilters.OpEqual, Val: "paid"}},
	})
	if err != nil || len(orders) != 1 || orders[0].Customer != nil {
		t.Fatalf("Get returned %v, %v, want the paid order without related objects", orders, err)
	}

	if err := crud.Delete(ctx, loaded, DeleteOptions{}); err != nil {
		t.Fatalf("Delete failed: %s", err.Error())
	}
}

func testInvalidRelation(t *testing.T, crud *CRUD) {
	if err := CreateTable[RelInvalid](context.Background(), crud); !errors.Is(err, structinfo.ErrRelation) {
		t.Fatalf("CreateTable returned err %v, want %v", err, structinfo.ErrRelation)
	}
}

func TestRelations_Fields(t *testing.T)        { testRelationFields(t, testCRUD) }
func TestSQLite_Relations_Fields(t *testing.T) { testRelationFields(t, testCRUDSQLite) }

func TestRelations_Invalid(t *testing.T)        { testInvalidRelation(t, testCRUD) }
func TestSQLite_Relations_Invalid(t *testing.T) { testInvalidRelation(t, testCRUDSQLite) }

func TestValidate_SkipsRelations(t *testing.T) {
	order := &RelOrder{Status: "placed", Customer: &RelCustomer{}, Items: []*RelItem{{}}}

	ok, violations, err := Validate(order, nil, "crud")
	if err != nil || !ok {
		t.Fatalf("Validate returned %v, %v, %v, want the related objects to be skipped", ok, violations, err)
	}

	ok, violations, err = Validate(order, map[string]bool{"Status": true, "Customer": true}, "crud")
	if err != nil || !ok {
		t.Fatalf("Validate with restricted fields returned %v, %v, %v, want the related objects to be skipped", ok, violations, err)
	}

	ok, violations, _ = Validate(&RelCustomer{Orders: []RelOrder{{}}}, nil, "crud")
	if ok || len(violations) != 1 || violations["Name"] != "required" {
		t.Fatalf("Validate returned %v, %v, want only the field of the customer to be validated", ok, violations)
	}
}
