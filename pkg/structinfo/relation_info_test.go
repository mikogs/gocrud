package structinfo

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

type relCustomer struct {
	ID     uint64
	Name   string
	Orders []relOrder `crud:"rel:CustomerID"`
}

type relOrder struct {
	ID         uint64
	CustomerID uint64
	ShipperID  *uint64
	Customer   *relCustomer    `crud:"rel:CustomerID"`
	Shipper    *relShipper     `crud:"rel:ShipperID"`
	Items      []relOrderItem  `crud:"rel:OrderID"`
	Notes      []*relOrderNote `crud:"rel:OrderID"`
	Tags       []string
}

type relShipper struct {
	ID   uint64
	Name string
}

type relOrderItem struct {
	ID      uint64
	OrderID uint64
}

type relOrderNote struct {
	ID      uint64
	OrderID *uint64
}

type relCategory struct {
	ID       uint64
	ParentID *uint64
	Parent   *relCategory  `crud:"rel:ParentID"`
	Children []relCategory `crud:"rel:ParentID"`
}

func TestNew_Relations(t *testing.T) {
	info := mustNew(t, &relOrder{}, "crud")

	want := []*RelationInfo{
		{Field: "Customer", Kind: BelongsTo, KeyField: "CustomerID", Type: reflect.TypeOf(relCustomer{})},
		{Field: "Shipper", Kind: BelongsTo, KeyField: "ShipperID", Type: reflect.TypeOf(relShipper{})},
		{Field: "Items", Kind: HasMany, KeyField: "OrderID", Type: reflect.TypeOf(relOrderItem{})},
		{Field: "Notes", Kind: HasMany, KeyField: "OrderID", Type: reflect.TypeOf(relOrderNote{}), PointerElements: true},
	}
	if !reflect.DeepEqual(info.Relations, want) {
		t.Fatalf("New returned relations %+v, want %+v", info.Relations, want)
	}

	wantColumns := []string{"ID", "CustomerID", "ShipperID"}
	if !reflect.DeepEqual(info.ColumnFieldNames, wantColumns) {
		t.Fatalf("New returned column field names %v, want %v", info.ColumnFieldNames, wantColumns)
	}

	for _, obj := range []interface{}{&relCustomer{}, &relCategory{}} {
		if _, err := New(obj, "crud"); err != nil {
			t.Fatalf("New failed for %T: %s", obj, err.Error())
		}
	}
}

func TestNew_InvalidRelations(t *testing.T) {
	type noID struct{ Name string }
	type wrongID struct {
		ID   int64
		Name string
	}
	type ignoredKey struct {
		ID      uint64
		OrderID uint64 `crud:"-"`
	}
	type stringKey struct {
		ID      uint64
		OrderID string
	}

	tests := map[string]interface{}{
		"on a column": &struct {
			ID         uint64
			CustomerID uint64 `crud:"rel:CustomerID"`
		}{},
		"on a time": &struct {
			ID        uint64
			ShippedAt *time.Time `crud:"rel:ID"`
		}{},
		"on a map": &struct {
			ID    uint64
			Name  string
			Items map[uint64]relOrderItem `crud:"rel:OrderID"`
		}{},
		"on a slice of strings": &struct {
			ID   uint64
			Name string
			Tags []string `crud:"rel:OrderID"`
		}{},
		"with another option": &struct {
			ID         uint64
			CustomerID uint64
			Customer   *relCustomer `crud:"rel:CustomerID idx"`
		}{},
		"without a key field": &struct {
			ID         uint64
			CustomerID uint64
			Customer   *relCustomer `crud:"rel:"`
		}{},
		"unexported": &struct {
			ID         uint64
			CustomerID uint64
			customer   *relCustomer `crud:"rel:CustomerID"`
		}{},
		"related struct without ID": &struct {
			ID    uint64
			Name  string
			Items []noID `crud:"rel:OrderID"`
		}{},
		"related struct with ID of a wrong type": &struct {
			ID    uint64
			Name  string
			Items []wrongID `crud:"rel:OrderID"`
		}{},
		"belongs to without the key field": &struct {
			ID       uint64
			Name     string
			Customer *relCustomer `crud:"rel:CustomerID"`
		}{},
		"belongs to with a key field of a wrong type": &struct {
			ID         uint64
			CustomerID int64
			Customer   *relCustomer `crud:"rel:CustomerID"`
		}{},
		"belongs to with an ignored key field": &struct {
			ID         uint64
			Name       string
			CustomerID uint64       `crud:"-"`
			Customer   *relCustomer `crud:"rel:CustomerID"`
		}{},
		"has many without the key field": &struct {
			ID    uint64
			Name  string
			Items []relOrderItem `crud:"rel:ParentID"`
		}{},
		"has many with an ignored key field": &struct {
			ID    uint64
			Name  string
			Items []ignoredKey `crud:"rel:OrderID"`
		}{},
		"has many with a key field of a wrong type": &struct {
			ID    uint64
			Name  string
			Items []stringKey `crud:"rel:OrderID"`
		}{},
	}

	for name, obj := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := New(obj, "crud"); !errors.Is(err, RelationError) {
				t.Fatalf("New returned err %v, want %v", err, RelationError)
			}
		})
	}
}

func TestRelationFieldNames(t *testing.T) {
	got := RelationFieldNames(reflect.TypeOf(relOrder{}), "crud")
	want := []string{"Customer", "Shipper", "Items", "Notes"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RelationFieldNames returned %v, want %v", got, want)
	}

	if got := RelationFieldNames(reflect.TypeOf(relOrder{}), "db"); len(got) != 0 {
		t.Fatalf("RelationFieldNames returned %v for another tag name, want none", got)
	}
}
